// tlsremover is a tls server that accepts ssl connections, strips the
// encryption and forwards the connection locally. does not do buffering though
// so tlsremover drops the connection if a side cannot keep up with the writes
// or if the local server sends data before the handshake finishes.

#define _GNU_SOURCE
#include <ctype.h>
#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <netinet/ip.h>
#include <netinet/tcp.h>
#include <openssl/err.h>
#include <openssl/ssl.h>
#include <signal.h>
#include <stdarg.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/epoll.h>
#include <sys/time.h>
#include <time.h>
#include <unistd.h>

#define nil ((void *)0)

// check is like an assert but always enabled.
#define check(cond) checkfunc(cond, #cond, __FILE__, __LINE__)
void checkfunc(bool ok, const char *s, const char *file, int line);

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
  char buf[90];
  va_list ap;
  int len;
  va_start(ap, fmt);
  len = vsnprintf(buf, 90, fmt, ap);
  va_end(ap);
  if (len > 80) {
    buf[77] = '.';
    buf[78] = '.';
    buf[79] = '.';
    buf[80] = 0;
    len = 80;
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
  if (errno != 0) log("errno: %m");
  log("ssl error: %s", ERR_reason_error_string(ERR_get_error()));
  exit(1);
}

enum { maxfd = 1000 };
enum { buffersize = 256 * 1024 };

enum fdtype { fdtypestraight, fdtypetls, fdtypeserver };
enum sslstate { sslstateaccept, sslstatenormal, sslstateshutdown };

struct fddata {
  enum fdtype type;
  int thisfd, pairfd;
  int fromport, toport;
  int64_t starttimems;
  SSL *ssl;
  enum sslstate sslstate;
};

struct s {
  struct fddata fds[maxfd + 1];
  char buf[buffersize];
} s;

int main(int argc, char **argv) {
  int epollfd;
  int i, fd, r, rby, wby;
  int fromfd, tofd;
  int fromport, toport;
  bool printusage;
  struct sockaddr_in addr;
  struct fddata *fdp;
  struct fddata *sslfdp, *straightfdp;
  struct epoll_event ev;
  struct timeval timeval;
  int64_t curtimems, deltams;
  SSL_CTX *ctx;
  const SSL_METHOD *method;
  const char *reason;
  const char *privkey, *certpem;

  // initialize state.
  privkey = nil;
  certpem = nil;
  signal(SIGPIPE, SIG_IGN);
  for (i = 0; i <= maxfd; i++) s.fds[i].thisfd = i;
  check((epollfd = epoll_create1(0)) != 0);
  ev.events = EPOLLIN;
  SSL_load_error_strings();
  ERR_load_crypto_strings();
  OpenSSL_add_ssl_algorithms();
  method = SSLv23_server_method();
  check((ctx = SSL_CTX_new(method)) != nil);
  SSL_CTX_set_ecdh_auto(ctx, 1);

  // process command line arguments.
  addr.sin_family = AF_INET;
  addr.sin_addr.s_addr = INADDR_ANY;
  printusage = false;
  while ((r = getopt(argc, argv, "c:p:")) != -1) {
    switch (r) {
    case 'c':
      certpem = optarg;
      break;
    case 'p':
      privkey = optarg;
      break;
    default:
      printusage = true;
      goto maybeprintusage;
    }
  }
  if (privkey == nil || certpem == nil) {
    puts("missing private key or certificate.");
    printusage = true;
    goto maybeprintusage;
  }
  check(SSL_CTX_use_certificate_chain_file(ctx, certpem) == 1);
  check(SSL_CTX_use_PrivateKey_file(ctx, privkey, SSL_FILETYPE_PEM) == 1);
  if (optind >= argc) printusage = true;
  for (i = optind; i < argc; i++) {
    if (sscanf(argv[i], "%d:%d", &fromport, &toport) != 2) {
      printf("bad argument %s\n", argv[i]);
      printusage = true;
      goto maybeprintusage;
    }
    check(1 <= fromport && fromport <= 65535);
    check(1 <= toport && toport <= 65535);
    check((fd = socket(AF_INET, SOCK_STREAM, 0)) != -1);
    check(fd <= maxfd);
    r = 1;
    check(setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &r, sizeof(r)) == 0);
    r = 10;
    check(setsockopt(fd, IPPROTO_TCP, TCP_DEFER_ACCEPT, &r, sizeof(r)) == 0);
    addr.sin_port = htons(fromport);
    check(bind(fd, &addr, sizeof(addr)) == 0);
    check(listen(fd, 10) == 0);
    ev.data.fd = fd;
    check(epoll_ctl(epollfd, EPOLL_CTL_ADD, fd, &ev) == 0);
    fdp = &s.fds[fd];
    fdp->type = fdtypeserver;
    fdp->fromport = fromport;
    fdp->toport = toport;
  }
  maybeprintusage:
  if (printusage) {
    puts("usage: tlsremover [args] [fromport1:toport1] ...");
    puts("tlsremover is a tls server that accepts connections and forwards");
    puts("them unencrypted.");
    puts("");
    puts("-c file.pem: the server's certificate.");
    puts("-p file.pem: the server's private key.");
    exit(1);
  }

  // main loop: accept connections and transmit data.
  log("server started");
  while (true) {
    check((r = epoll_wait(epollfd, &ev, 1, -1)) == 1);
    gettimeofday(&timeval, nil);
    curtimems = timeval.tv_sec * 1000ll + timeval.tv_usec / 1000;
    fdp = &s.fds[ev.data.fd];
    if (fdp->type == fdtypeserver) {
      fromport = fdp->fromport;
      toport = fdp->toport;
      fromfd = accept4(fdp->thisfd, nil, nil, SOCK_NONBLOCK);
      check(fromfd != -1);
      if (fromfd > maxfd) {
        log("rejected on port %d due to overload", fromport);
        check(close(fromfd) == 0);
        continue;
      }
      i = buffersize;
      check(setsockopt(fromfd, SOL_SOCKET, SO_SNDBUF, &i, sizeof(i)) == 0);
      check((tofd = socket(AF_INET, SOCK_STREAM, 0)) != -1);
      if (tofd > maxfd) {
        log("rejected on port %d due to overload", fromport);
        check(close(fromfd) == 0);
        check(close(tofd) == 0);
        continue;
      }
      i = buffersize;
      check(setsockopt(tofd, SOL_SOCKET, SO_SNDBUF, &i, sizeof(i)) == 0);
      addr.sin_port = htons(toport);
      r = connect(tofd, &addr, sizeof(addr));
      if (r == -1) {
        log("rejected on port %d because %d is unavailable", fromport, toport);
        check(close(fromfd) == 0);
        check(close(tofd) == 0);
        continue;
      }
      ev.data.fd = fromfd;
      check(epoll_ctl(epollfd, EPOLL_CTL_ADD, fromfd, &ev) == 0);
      ev.data.fd = tofd;
      check(epoll_ctl(epollfd, EPOLL_CTL_ADD, tofd, &ev) == 0);
      check(r = fcntl(tofd, F_GETFL, 0) != -1);
      check(fcntl(fd, F_SETFL, r | O_NONBLOCK) == 0);
      s.fds[fromfd].type = fdtypetls;
      s.fds[fromfd].sslstate = sslstateaccept;
      s.fds[fromfd].pairfd = tofd;
      s.fds[fromfd].fromport = fromport;
      s.fds[fromfd].starttimems = curtimems;
      s.fds[tofd].type = fdtypestraight;
      s.fds[tofd].pairfd = fromfd;
      s.fds[tofd].fromport = fromport;
      s.fds[tofd].starttimems = curtimems;
      fdp = &s.fds[fromfd];
      check((fdp->ssl = SSL_new(ctx)) != nil);
      check(SSL_set_fd(fdp->ssl, fromfd) == 1);
    }
    // fdp now contains the socket with read data waiting.
    fromport = fdp->fromport;
    if (fdp->type == fdtypestraight) {
      rby = read(fdp->thisfd, s.buf, buffersize);
      if (rby == -1 && (errno == EAGAIN || errno == EINTR)) continue;
    } else {
      check(fdp->type == fdtypetls);
      if (fdp->sslstate == sslstateaccept) {
        r = SSL_accept(fdp->ssl);
        if (r == 0) {
          reason = "ssl accept returned 0";
          goto closeconnections;
        }
        if (r == 1) {
          fdp->sslstate = sslstatenormal;
          continue;
        }
        check(r < 0);
        if (SSL_get_error(fdp->ssl, r) == SSL_ERROR_WANT_READ) continue;
        reason = "ssl accept returned error";
        goto closeconnections;
      }
      if (fdp->sslstate == sslstateshutdown) {
        r = SSL_shutdown(fdp->ssl);
        if (r == 1) {
          reason = "success";
          goto closeconnections;
        }
        check(r < 0);
        if (SSL_get_error(fdp->ssl, r) == SSL_ERROR_WANT_READ) continue;
        reason = "second ssl shutdown returned error";
        goto closeconnections;
      }
      check(fdp->sslstate == sslstatenormal);
      rby = SSL_read(fdp->ssl, s.buf, buffersize);
      if (rby < 0) {
        if (SSL_get_error(fdp->ssl, rby) == SSL_ERROR_WANT_READ) continue;
        reason = "ssl read returned error";
        goto closeconnections;
      }
    }
    // the read data is in s.buf and the amount of data is in rby. shut down the
    // connections on eof.
    if (rby == 0) {
      if (fdp->type == fdtypetls) {
        sslfdp = fdp;
        straightfdp = &s.fds[fdp->pairfd];
        check(straightfdp->type == fdtypestraight);
      } else {
        straightfdp = fdp;
        sslfdp = &s.fds[fdp->pairfd];
        check(sslfdp->type == fdtypetls);
      }
      check(epoll_ctl(epollfd, EPOLL_CTL_DEL, straightfdp->thisfd, nil) == 0);
      sslfdp->sslstate = sslstateshutdown;
      r = SSL_shutdown(sslfdp->ssl);
      if (r == 0) continue;
      if (r == 1) {
        reason = "success";
        goto closeconnections;
      }
      check(r < 0);
      if (SSL_get_error(fdp->ssl, r) == SSL_ERROR_WANT_READ) continue;
      reason = "ssl shutdown returned error";
      goto closeconnections;
    }
    check(rby > 0);
    // now write out the rby long data in s.buf.
    if (fdp->type == fdtypetls) {
      wby = write(fdp->pairfd, s.buf, rby);
    } else {
      check(fdp->type == fdtypestraight);
      wby = SSL_write(s.fds[fdp->pairfd].ssl, s.buf, rby);
    }
    if (wby != rby) {
      log("wrote %d out of %d", wby, rby);
      reason = "partial write";
      goto closeconnections;
    }
    continue;
    closeconnections:
    deltams = curtimems - fdp->starttimems;
    log("finished on port %d after %lld ms: %s", fromport, deltams, reason);
    if (strcmp(reason, "success") != 0) {
      log("last ssl error: %s", ERR_reason_error_string(ERR_get_error()));
    }
    if (fdp->type == fdtypestraight) {
      fdp = &s.fds[fdp->pairfd];
      check(fdp->type == fdtypetls);
    }
    SSL_free(fdp->ssl);
    check(close(fdp->thisfd) == 0);
    check(close(fdp->pairfd) == 0);
  }
  EVP_cleanup();
  return 0;
}
