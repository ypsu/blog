// webserve is a very basic static web server to serve very small files. it has
// the bare minimum set of features to be able concurrently serve a set of
// static files. start the webserver in the directory you want to serve the
// files from and webserve will read everything into the memory and then serve
// those from memory. it does not traverse or list directories. use ctrl-c to
// refresh its contents cache without a restart.
//
// it works by avoiding storing any per connection data. the listen sockets have
// the TCP_DEFER_ACCEPT setting on so the request line is immediately known. it
// also assumes that the responses fit into the kernel's socket buffer.
// therefore webserve handles each accept by immediately calling read and write
// on the connection. obviously only use to serve non-critical data.
//
// the landing page, aka root request ("/") goes to the file "gopherindex" in
// gopher mode. in http mode it goes to the file set via the -m option or serves
// 404 if the request lacks a hostname.
//
// over time it acquired a few other features:
//
// - per domain name landing pages (see the -m option).
// - support for acme challenges (use -a to configure the path).
// - can act as a barebones signaling server for webrtc (see the big comment
//   just above the sigdata struct).

#define _GNU_SOURCE
#include <ctype.h>
#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <netinet/ip.h>
#include <netinet/tcp.h>
#include <signal.h>
#include <stdarg.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/epoll.h>
#include <sys/mman.h>
#include <sys/time.h>
#include <time.h>
#include <unistd.h>

#define nil ((void *)0)

// check is like an assert but always enabled.
#define check(cond) checkfunc(cond, #cond, __FILE__, __LINE__)
void checkfunc(bool ok, const char *s, const char *file, int line);

// state is a succinct string representation of what the application is
// currently doing. it's never logged. it's only helpful to quickly identify why
// something crashed because checkfail prints its contents.
enum { statelen = 200 };
char state[statelen + 1];
__attribute__((format(printf, 1, 2)))
void setstatestr(const char *fmt, ...) {
  va_list ap;
  va_start(ap, fmt);
  vsnprintf(state, statelen, fmt, ap);
  va_end(ap);
}

// log usage: log("started server on port %d", port);
#define log loginfo(__FILE__, __LINE__), logfunc
void loginfo(const char *file, int line) {
  struct timeval tv;
  struct tm *tm;
  char buf[30];
  gettimeofday(&tv, nil);
  tm = gmtime(&tv.tv_sec);
  check(strftime(buf, 30, "%F %H:%M:%S", tm) > 0);
  printf("%s.%03d %s:%d ", buf, (int)tv.tv_usec / 1000, file, line);
}
__attribute__((format(printf, 1, 2)))
void logfunc(const char *fmt, ...) {
  char buf[200];
  va_list ap;
  int len;
  va_start(ap, fmt);
  len = vsnprintf(buf, 200, fmt, ap);
  va_end(ap);
  if (len > 198) {
    buf[194] = '.';
    buf[196] = '.';
    buf[197] = '.';
    buf[198] = 0;
    len = 198;
  }
  for (int i = 0; i < len; i++) {
    if (32 <= buf[i] && buf[i] <= 127) continue;
    buf[i] = '.';
  }
  puts(buf);
}

void checkfunc(bool ok, const char *s, const char *file, int line) {
  if (ok) return;
  loginfo(file, line);
  logfunc("checkfail %s", s);
  log("state: %s", state);
  if (errno != 0) log("errno: %m");
  exit(1);
}

// maxnamelength is the maximum length a filename can have.
enum { maxnamelength = 31 };

// maxfiles is the number of different files the server can handle.
enum { maxfiles = 1000 };

// number of virtual hosts this server can handle.
enum { maxhosts = 4 };

// buffersize is used to size the two temporary helper buffers. it's big enough
// to handle all the various usecases.
enum { buffersize = 1024 * 1024 };

// number of signals this server maintains.
enum { sigcount = 10 };

// signamelimit includes the null terminator.
enum { signamelimit = 16 };

// the maximum content size for a signal.
enum { sigcontentlimit = 1000 };

enum { sigtimeoutsecs = 600 };

// filedata contains the full http response for each file.
struct filedata {
  int length;
  // dataoffset contains the offset at which the raw data begins. handy for the
  // gopher responses which don't have any headers.
  int dataoffset;
  // data is a malloc'd pointer. this structure owns it.
  char *data;
  char name[maxnamelength + 1];
};

int filedatacmp(const void *a, const void *b) {
  const struct filedata *x = a;
  const struct filedata *y = b;
  return strcmp(x->name, y->name);
}

// the following datastructure aids implementing a rudimentary signaling server
// for webrtc demos. in this server's context a signal is a named, temporary
// message that can be read only once. after reading it, the server destroys the
// message. the content of such message is limited to be at most 1000 bytes
// long. a signal's name must match the "[a-z0-9]{1,15}" regex (without the
// quotes). the following operations are allowed (get and post refer to the http
// methods and post must be sent in plaintext mode and must be sent along the
// first packet so that the server can read with a single read with
// TCP_DEFER_ACCEPT):
//
// - post /sigset/name: set the content of a signal, returns immediately.
// - get /sigquery/name: query the content of a signal, returns immediately.
//   returns 204 if there is no such signal is available.
// - get /sigget/name: query the content of a signal. if there is no such
//   signal, wait until it appears. hence this is a blocking operation. can also
//   return 408 for timeout or 409 if another sigget query replaced the current
//   one.
// - get /sigwait/name: wait until a signal disappears. returns 204 if the
//   signal is already non-existent, 200 for success, 408 for timeout and 409
//   for a reset.
//
// this implementation is not intended for heavy duty use, only for demos, so it
// can only deal with a handful of signals. and note that each signal can have
// at most 1 listener (one connection waiting for the status change).
//
// example usage:
//
// client 1:
//   curl notech.ie/sigget/example
//
// client 2:
//   curl --data "this is an example signal" notech.ie/sigset/example
//
// here client 1 will block until client 2 runs its command.
//
// s.signal array contains a dozen instances of this struct. the server
// implements each operation by iterating through all the array elements to find
// a matching signal (or find an empty entry for the set operations).
//
// here is how the above 4 operations can be used in an imaginary webrtc
// chat application (assuming familiarity with webrtc offer/answer parlace):
//
// - when the user loads the web app, the app checks with /sigquery/chatoffer if
//   there is already a server running somewhere and if so accepts that offer
//   and acts as a client.
// - otherwise it becomes a server and uploads an offer via /sigset/chatoffer
//   and starts waiting for an answer via /sigget/chatanswer (this is a blocking
//   call).
// - meanwhile the server also issues a /sigwait/chatoffer in the background.
//   upon returning, the server uploads another chatoffer via /sigset/chatoffer.
//   this is to handle the case where a client reads the offer but does not
//   answer - we still want other clients to be able to connect.
// - if the app is client, then it returns an answer with /sigset/chatanswer.
// - the server receives the answer via /sigget/chatanswer and establishes the
//   connection via the webrtc api.
//
// in case the server handles multiple connections, it needs to make sure it
// pairs the offers and answers correctly since they could in theory come out of
// order or not come at all (although this is pretty unlikely, probably not
// worth the care in demos). the app could use the id and label attributes on
// the rtcdatachannel to set an id that the server can use for identification
// (e.g. the id could be the index into a connection array).

struct sigdata {
  char name[signamelimit];
  // the time when the content was set. a signal is considered non-existent
  // after 10 minutes of inactivity.
  time_t added;
  // fd of a waiting socket if any. 0 otherwise.
  int fd;
  // length of the content buffer below. 0 if there is no content yet.
  int len;
  // this contains the signal's message (not a null terminated string).
  char content[sigcontentlimit];
};

struct {
  // if reloadfiles is true, webserve will reload all files from disk during the
  // next iteration of the main loop.
  bool reloadfiles;
  int filescount;
  struct filedata files[maxfiles];
  // contains the index page for the various hostnames. e.g. "notech.ie" ->
  // "frontpage".
  char hostmapping[maxhosts][2][maxnamelength + 1];

  // helper buffers.
  char buf1[buffersize], buf2[buffersize];

  // marks the fact that the user pressed ctrl-c.
  bool interrupted;

  // contents of the various signals people can upload.
  struct sigdata signal[sigcount];
} s;

void siginthandler(int sig) {
  if (sig == SIGINT) {
    s.interrupted = true;
  }
}

// this function closes fd as well.
void writeplaincontent(int fd, char *content, int len) {
  char *p = s.buf2;
  p += sprintf(p, "HTTP/1.1 200 OK\r\n");
  p += sprintf(p, "Content-Type: text/plain; charset=utf-8\r\n");
  p += sprintf(p, "Connection: close\r\n");
  p += sprintf(p, "Content-Length: %d\r\n", len);
  p += sprintf(p, "Access-Control-Allow-Origin: *\r\n");
  p += sprintf(p, "\r\n");
  memcpy(p, content, len);
  p += len;
  write(fd, s.buf2, p - s.buf2);
  check(close(fd) == 0);
}

const char httpnotfound[] =
  "HTTP/1.1 404 Not Found\r\n"
  "Content-Type: text/plain; charset=utf-8\r\n"
  "Connection: close\r\n"
  "Content-Length: 14\r\n"
  "Access-Control-Allow-Origin: *\r\n"
  "\r\n"
  "404 not found\n";
const char httpnotimpl[] =
  "HTTP/1.1 501 Not Implemented\r\n"
  "Content-Type: text/plain; charset=utf-8\r\n"
  "Connection: close\r\n"
  "Content-Length: 20\r\n"
  "Access-Control-Allow-Origin: *\r\n"
  "\r\n"
  "501 not implemented\n";
const char sigsuccessmsg[] =
  "HTTP/1.1 200 OK\r\n"
  "Content-Type: text/plain; charset=utf-8\r\n"
  "Connection: close\r\n"
  "Content-Length: 0\r\n"
  "Access-Control-Allow-Origin: *\r\n"
  "\r\n\r\n";
const char sigmissingmsg[] =
  "HTTP/1.1 204 No Content\r\n"
  "Content-Type: text/plain; charset=utf-8\r\n"
  "Connection: close\r\n"
  "Content-Length: 0\r\n"
  "Access-Control-Allow-Origin: *\r\n"
  "\r\n\r\n";
const char sigtimeoutmsg[] =
  "HTTP/1.1 408 Request Timeout\r\n"
  "Connection: close\r\n"
  "Content-Length: 0\r\n"
  "Access-Control-Allow-Origin: *\r\n"
  "\r\n\r\n";
const char sigconflictmsg[] =
  "HTTP/1.1 409 Conflict\r\n"
  "Connection: close\r\n"
  "Content-Length: 0\r\n"
  "Access-Control-Allow-Origin: *\r\n"
  "\r\n\r\n";

int main(int argc, char **argv) {
  // never swap out this process to ensure it's always fast.
  check(mlockall(MCL_CURRENT | MCL_FUTURE) == 0);

  // variables for single purpose.
  int httpfd, gopherfd, epollfd;
  int maxfilesize = 100 * 1000;
  const char *acmepath = nil;

  // helper variables with many different uses.
  int i, r, port, opt, fd, len, acmefd, ch;
  int lo, hi, mid;
  long long totalbytes;
  char *pbuf;
  const char *type;
  struct sockaddr_in addr;
  struct epoll_event ev;
  DIR *dir;
  struct dirent *dirent;
  struct filedata *fdp;
  time_t curtime, lastinterrupt;

  // initialize data.
  setstatestr("initializing");
  httpfd = -1;
  gopherfd = -1;
  s.reloadfiles = true;
  lastinterrupt = 0;
  check(signal(SIGINT, siginthandler) == SIG_DFL);
  check(signal(SIGPIPE, SIG_IGN) == SIG_DFL);

  // parse cmdline arguments.
  addr.sin_family = AF_INET;
  addr.sin_addr.s_addr = INADDR_ANY;
  while ((opt = getopt(argc, argv, "a:g:hl:m:p:")) != -1) {
    switch (opt) {
    case 'a':
      acmepath = optarg;
      break;
    case 'g':
      port = atoi(optarg);
      check(1 <= port && port <= 65535);
      check((gopherfd = socket(AF_INET, SOCK_STREAM, 0)) != -1);
      i = 1;
      check(setsockopt(gopherfd, SOL_SOCKET, SO_REUSEADDR, &i, sizeof(i)) == 0);
      i = 10;
      r = setsockopt(gopherfd, IPPROTO_TCP, TCP_DEFER_ACCEPT, &i, sizeof(i));
      check(r == 0);
      addr.sin_port = htons(port);
      check(bind(gopherfd, &addr, sizeof(addr)) == 0);
      break;
    case 'm':
      for (i = 0; i < maxhosts && s.hostmapping[i][0][0] != 0; i++);
      check(i < maxhosts);
      check(strlen(optarg) <= maxnamelength);
      const char fmt[] = "%[a-z.]:%s";
      check(sscanf(optarg, fmt, s.hostmapping[i][0], s.hostmapping[i][1]) == 2);
      break;
    case 'p':
      port = atoi(optarg);
      check(1 <= port && port <= 65535);
      check((httpfd = socket(AF_INET, SOCK_STREAM, 0)) != -1);
      i = 1;
      check(setsockopt(httpfd, SOL_SOCKET, SO_REUSEADDR, &i, sizeof(i)) == 0);
      i = 10;
      r = setsockopt(httpfd, IPPROTO_TCP, TCP_DEFER_ACCEPT, &i, sizeof(i));
      addr.sin_port = htons(port);
      check(bind(httpfd, &addr, sizeof(addr)) == 0);
      break;
    case 'l':
      maxfilesize = atoi(optarg);
      check(1 <= maxfilesize && maxfilesize + 1024 < buffersize);
      break;
    case 'h':
    default:
      puts("serves the small files from the current directory.");
      puts("usage: webserve [args]");
      puts("");
      puts("flags:");
      puts("  -a path: path to let's encrypt acme challenges");
      puts("  -g port: start gopher server on port");
      puts("  -l size: file size limit. default is 100 kB. make sure the");
      puts("           kernel can buffer this amount of data.");
      puts("  -m map:  sets the landing page for the various hostnames.");
      puts("           e.g. set map to \"notech.ie:frontpage\" to serve");
      puts("           \"frontpage\" as the landing page for notech.ie/.");
      puts("  -p port: start http server on port");
      puts("  -h: this help message");
      exit(1);
    }
  }
  if (httpfd == -1 && gopherfd == -1) {
    puts("please specify the port number to listen on.");
    exit(1);
  }

  // set up the server sockets.
  log("server pid %d", (int)getpid());
  check((epollfd = epoll_create1(0)) != -1);
  ev.events = EPOLLIN;
  if (httpfd != -1) {
    check(listen(httpfd, 100) == 0);
    ev.data.fd = httpfd;
    check(epoll_ctl(epollfd, EPOLL_CTL_ADD, httpfd, &ev) == 0);
    log("http server started");
  }
  if (gopherfd != -1) {
    check(listen(gopherfd, 100) == 0);
    ev.data.fd = gopherfd;
    check(epoll_ctl(epollfd, EPOLL_CTL_ADD, gopherfd, &ev) == 0);
    log("gopher server started");
  }

  // run the event loop.
  while (true) {
    // reload files if necessary.
    if (s.reloadfiles) {
      setstatestr("reloading files");
      s.reloadfiles = false;
      for (i = 0; i < maxfiles; i++) {
        free(s.files[i].data);
      }
      totalbytes = 0;
      s.filescount = 0;
      memset(s.files, 0, sizeof(s.files));
      check((dir = opendir(".")) != nil);
      while ((dirent = readdir(dir)) != nil) {
        if (s.filescount == maxfiles) {
          log("too many files in directory, ignoring the rest");
          break;
        }
        if (dirent->d_type == DT_DIR) {
          continue;
        }
        len = strlen(dirent->d_name);
        if (len > maxnamelength) {
          log("skipped the long named %s", dirent->d_name);
          continue;
        }
        setstatestr("reloading %s", dirent->d_name);
        fd = open(dirent->d_name, O_RDONLY);
        check(fd != -1);
        len = read(fd, s.buf1, maxfilesize + 1);
        check(len != -1);
        if (len <= maxfilesize) {
          check(read(fd, s.buf1, 1) == 0);
        }
        check(close(fd) == 0);
        if (len > maxfilesize) {
          log("skipped oversized %s", dirent->d_name);
          continue;
        }
        pbuf = s.buf2;
        pbuf += sprintf(pbuf, "HTTP/1.1 200 OK\r\n");
        pbuf += sprintf(pbuf, "Content-Type: ");
        if (strncasecmp(s.buf1, "<!DOCTYPE html", 14) == 0) {
          pbuf += sprintf(pbuf, "text/html");
        } else if (memcmp(s.buf1, "<?xml", 5) == 0) {
          pbuf += sprintf(pbuf, "text/xml");
        } else if (memcmp(s.buf1, "\x89PNG", 4) == 0) {
          pbuf += sprintf(pbuf, "image/png");
        } else if (memcmp(s.buf1, "\xff\xd8\xff", 3) == 0) {
          pbuf += sprintf(pbuf, "image/jpeg");
        } else if (memcmp(s.buf1, "%PDF", 4) == 0) {
          pbuf += sprintf(pbuf, "application/pdf");
        } else {
          pbuf += sprintf(pbuf, "text/plain");
        }
        pbuf += sprintf(pbuf, "; charset=utf-8\r\n");
        pbuf += sprintf(pbuf, "Cache-Control: max-age=3600\r\n");
        pbuf += sprintf(pbuf, "Connection: close\r\n");
        pbuf += sprintf(pbuf, "Content-Length: %d\r\n", len);
        pbuf += sprintf(pbuf, "\r\n");
        fdp = &s.files[s.filescount++];
        fdp->length = len + pbuf - s.buf2;
        fdp->dataoffset = pbuf - s.buf2;
        check((fdp->data = malloc(fdp->length)) != nil);
        memcpy(fdp->data, s.buf2, fdp->dataoffset);
        memcpy(fdp->data + fdp->dataoffset, s.buf1, len);
        strcpy(fdp->name, dirent->d_name);
        totalbytes += fdp->length;
      }
      check(closedir(dir) == 0);
      log("loaded %d files, cache is %lld bytes", s.filescount, totalbytes);
      qsort(s.files, s.filescount, sizeof(s.files[0]), filedatacmp);
    }

    // process a socket event.
    setstatestr("waiting for events");
    r = epoll_wait(epollfd, &ev, 1, -1);
    curtime = time(nil);
    if (r == -1 && errno == EINTR) {
      errno = 0;
      check(s.interrupted);
      s.interrupted = false;
      s.reloadfiles = true;
      if (curtime - lastinterrupt <= 2) {
        log("quitting");
        exit(0);
      }
      lastinterrupt = curtime;
      log("interrupt received, press again to quickly to quit");
      continue;
    }
    check(r == 1);
    // accept and read the request into s.buf1. pbuf will point at the response
    // eventually and its length will be len.
    setstatestr("accepting a request");
    fd = accept4(ev.data.fd, nil, nil, SOCK_NONBLOCK);
    i = maxfilesize + 1024;
    check(setsockopt(fd, SOL_SOCKET, SO_SNDBUF, &i, sizeof(i)) == 0);
    len = read(fd, s.buf1, buffersize - 1);
    if (len <= 0) {
      log("responded 501 because read returned %d (errno: %m)", len);
      goto notimplementederror;
    }
    s.buf1[len] = 0;
    pbuf = s.buf1;
    if (ev.data.fd == httpfd) {
      // extract content if the query has one.
      type = "http";
      int contentlen = 0;
      char *content, *tmp;
      tmp = strstr(pbuf, "Content-Length:");
      if (tmp != nil) {
        sscanf(tmp, "Content-Length: %d", &contentlen);
        if (contentlen > sigcontentlimit) {
          log("too big content for http request %s", pbuf);
          goto notimplementederror;
        }
        tmp = strstr(pbuf, "\r\n\r\n");
        if (tmp == nil) {
          log("missing content for http request %s", pbuf);
          goto notimplementederror;
        }
        tmp += 4;
        r = len - (tmp - pbuf);
        if (r != contentlen) {
          const char *fmt;
          fmt = "partial content (%d vs %d) for http request %s";
          log(fmt, r, contentlen, pbuf);
          goto notimplementederror;
        }
        content = tmp;
      }
      // handle the signaling server requests.
      char signame[32] = "";
      r = sscanf(pbuf, "%*s /sig%*[a-z]/%30s", signame);
      if (r == 1) {
        if (strlen(signame) > signamelimit - 1) {
          log("signal name %s too long", signame);
          goto notimplementederror;
        }
        // iterate through s.signal and find the corresponding signal element.
        struct sigdata *sig = nil;
        struct sigdata *sigempty = nil;
        for (int i = 0; i < sigcount; i++) {
          if (curtime - s.signal[i].added > sigtimeoutsecs) {
            // time out open, waiting connection if any.
            if (s.signal[i].fd != 0) {
              const char *resp;
              int resplen;
              if (s.signal[i].len == 0) {
                // fd represents a /sigget request.
                resp = sigtimeoutmsg;
                resplen = sizeof(sigtimeoutmsg) - 1;
              } else {
                // fd represends a /sigwait request.
                resp = sigmissingmsg;
                resplen = sizeof(sigmissingmsg) - 1;
              }
              write(s.signal[i].fd, resp, resplen);
              check(close(s.signal[i].fd) == 0);
              s.signal[i].fd = 0;
            }
            s.signal[i].len = 0;
            s.signal[i].name[0] = 0;
          }
          if (strcmp(s.signal[i].name, signame) == 0) {
            sig = &s.signal[i];
            break;
          }
          if (sigempty == nil && s.signal[i].name[0] == 0) {
            sigempty = &s.signal[i];
          }
        }
        // process the queries.
        if (memcmp(pbuf, "GET /sigquery/", 14) == 0) {
          if (sig == nil) goto signotfounderror;
          if (sig->len == 0) goto signotfounderror;
          if (sig->fd != 0) {
            // fd is /sigwait.
            write(sig->fd, sigsuccessmsg, sizeof(sigsuccessmsg) - 1);
            check(close(sig->fd) == 0);
            sig->fd = 0;
          }
          log("queried signal %s", signame);
          writeplaincontent(fd, sig->content, sig->len);
          sig->len = 0;
          sig->name[0] = 0;
          goto nextiteration;
        }
        if (memcmp(pbuf, "POST /sigset/", 13) == 0) {
          if (contentlen == 0) goto notimplementederror;
          if (sig == nil) {
            if (sigempty == nil) {
              log("cannot add signal %s, too many active ones", signame);
              goto notimplementederror;
            }
            sig = sigempty;
          }
          if (sig->fd != 0) {
            if (sig->len == 0) {
              // new signal is being added, fd is a /sigget.
              writeplaincontent(sig->fd, content, contentlen);
              sig->fd = 0;
              sig->name[0] = 0;
              pbuf = (char *)sigsuccessmsg;
              len = sizeof(sigsuccessmsg) - 1;
              log("forwarded a signal %s", signame);
              goto respond;
            } else {
              // replacing a signal, fd is a /sigwait.
              pbuf = (char *)sigconflictmsg;
              len = sizeof(sigconflictmsg) - 1;
              write(sig->fd, pbuf, len);
              check(close(sig->fd) == 0);
              sig->fd = 0;
            }
          }
          strcpy(sig->name, signame);
          sig->added = curtime;
          sig->len = contentlen;
          memcpy(sig->content, content, contentlen);
          pbuf = (char *)sigsuccessmsg;
          len = sizeof(sigsuccessmsg) - 1;
          log("added signal %s", signame);
          goto respond;
        }
        if (memcmp(pbuf, "GET /sigget/", 12) == 0) {
          if (sig == nil) {
            if (sigempty == nil) {
              log("cannot wait signal %s, too many active ones", signame);
              goto notimplementederror;
            }
            sig = sigempty;
            strcpy(sig->name, signame);
            sig->added = curtime;
            sig->fd = fd;
            log("waiting on signal %s", signame);
            goto nextiteration;
          }
          if (sig->len == 0) {
            check(sig->fd != 0);
            log("replacing get on signal %s", signame);
            write(sig->fd, sigconflictmsg, sizeof(sigconflictmsg) - 1);
            check(close(sig->fd) == 0);
            sig->fd = fd;
            goto nextiteration;
          }
          check(sig->fd == 0);
          log("got signal %s", signame);
          writeplaincontent(fd, sig->content, sig->len);
          sig->len = 0;
          sig->name[0] = 0;
          goto nextiteration;
        }
        if (memcmp(pbuf, "GET /sigwait/", 13) == 0) {
          if (sig == nil) {
            goto signotfounderror;
          }
          if (sig->fd != 0) {
            log("replacing wait on signal %s", signame);
            write(sig->fd, sigmissingmsg, sizeof(sigmissingmsg) - 1);
            check(close(sig->fd) == 0);
          }
          sig->fd = fd;
          goto nextiteration;
        }
        goto notimplementederror;
signotfounderror:
        write(fd, sigmissingmsg, sizeof(sigmissingmsg) - 1);
        check(close(fd) == 0);
        goto nextiteration;
      }
      // only the ordinary get request for the blog content remains.
      if (memcmp(pbuf, "GET /", 5) != 0) {
        log("responded 501 because of unimplemented http request: %s", pbuf);
        goto notimplementederror;
      }
      pbuf += 5;
    } else if (ev.data.fd == gopherfd) {
      type = "gopher";
      if (*pbuf != '/') {
        log("responded 501 because of bad gopher request: %s", pbuf);
        goto notimplementederror;
      }
      pbuf++;
    } else {
      check(false);
    }
    if (isspace(*pbuf)) {
      if (ev.data.fd == gopherfd) {
        strcpy(s.buf2, "gopherindex");
      } else {
        s.buf2[0] = 0;
        for (i = 0; i < maxhosts && s.hostmapping[i][0][0] != 0; i++) {
          if (strstr(s.buf1, s.hostmapping[i][0]) != 0) {
            strcpy(s.buf2, s.hostmapping[i][1]);
            break;
          }
        }
      }
    } else {
      check(sscanf(pbuf, "%s%n", s.buf2, &len) == 1);
      pbuf += len;
      if (!isspace(*pbuf)) {
        log("responded 501 because of unfinished %s request: %s", type, s.buf1);
        goto notimplementederror;
      }
    }
    setstatestr("responding to %s /%s", type, s.buf2);
    // find the entry via binary search and respond.
    lo = 0, hi = s.filescount - 1;
    while (lo <= hi) {
      mid = (lo + hi) / 2;
      fdp = &s.files[mid];
      r = strcmp(fdp->name, s.buf2);
      if (r == 0) {
        pbuf = fdp->data;
        len = fdp->length;
        if (ev.data.fd == gopherfd) {
          pbuf += fdp->dataoffset;
          len -= fdp->dataoffset;
        }
        log("responded success to %s /%s", type, s.buf2);
        goto respond;
      } else if (r < 0) {
        lo = mid + 1;
      } else {
        hi = mid - 1;
      }
    }
    // check if the entry is an acme challenge and respond as such.
    if (acmepath == nil) goto notfounderror;
    if (memcmp(s.buf2, ".well-known/acme-challenge/", 27) != 0) {
      goto notfounderror;
    }
    for (i = 27; (ch = s.buf2[i]) != 0; i++) {
      if (ch == '.' || ch == '/') goto notfounderror;
    }
    if (snprintf(s.buf1, buffersize, "%s/%s", acmepath, s.buf2) >= buffersize) {
      goto notfounderror;
    }
    acmefd = open(s.buf1, O_RDONLY);
    if (acmefd == -1) goto notfounderror;
    r = read(acmefd, s.buf2, buffersize - 1000);
    check(r != -1);
    check(close(acmefd) == 0);
    pbuf = s.buf1;
    pbuf += sprintf(pbuf, "HTTP/1.1 200 OK\r\n");
    pbuf += sprintf(pbuf, "Content-Type: text/plain\r\n");
    pbuf += sprintf(pbuf, "Connection: close\r\n");
    pbuf += sprintf(pbuf, "Content-Length: %d\r\n", r);
    pbuf += sprintf(pbuf, "\r\n");
    pbuf += sprintf(pbuf, "%s", s.buf2);
    len = pbuf - s.buf1;
    pbuf = s.buf1;
    log("responded success to an acme challenge");
    goto respond;
notfounderror:
    log("responded 404 to %s /%s", type, s.buf2);
    if (ev.data.fd == httpfd) {
      pbuf = (char *)httpnotfound;
      len = sizeof(httpnotfound) - 1;
    } else {
      len = sprintf(s.buf1, "404 not found\n");
      pbuf = s.buf1;
    }
    goto respond;
notimplementederror:
    if (ev.data.fd == httpfd) {
      pbuf = (char *)httpnotimpl;
      len = sizeof(httpnotimpl) - 1;
    } else {
      len = sprintf(s.buf1, "501 not implemented\n");
      pbuf = s.buf1;
    }
    goto respond;
respond:
    r = write(fd, pbuf, len);
    if (r == -1 && (errno == ECONNRESET || errno == EPIPE)) {
      log("got a connection reset or broken pipe error");
    } else if (r != len) {
      log("the kernel can't buffer %d worth of bytes, only %d", len, r);
      check(false);
    } else {
      check(r == len);
    }
    check(close(fd) == 0);
nextiteration:;
  }

  return 0;
}
