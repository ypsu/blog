// this is a small tool to acquire cap_net_bind_service linux capability.
// then my little webservers can open the standard web ports
// without being root or passing along the open file descriptors.
// after building you need setcap the appropriate capability onto the binary.
// see the build script for the exact command.
//
// usage: bindcap [cmd] [args...]

#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/capability.h>
#include <sys/prctl.h>
#include <unistd.h>

// check is like an assert but always enabled.
#define check(cond) checkfunc(cond, #cond, __FILE__, __LINE__)
void checkfunc(bool ok, const char *s, const char *file, int line) {
  if (ok) return;
  printf("checkfail %s at %s:%d, errno: %m\n", s, file, line);
  exit(1);
}

int main(int argc, char **argv) {
  // print usage if needed.
  if (argc <= 1) {
    puts("usage: bindcap [cmd] [args...]");
    puts("runs cmd with cap_net_bind_service.");
    return 0;
  }

  // make cap_net_bind_service inheritable.
  cap_t caps = cap_get_proc();
  check(caps != NULL);
  cap_value_t newcaps[1] = {CAP_NET_BIND_SERVICE};
  check(cap_set_flag(caps, CAP_INHERITABLE, 1, newcaps, CAP_SET) == 0);
  if (cap_set_proc(caps) != 0) {
    printf("error in cap_set_proc: %m\n\n");
    puts("make sure to run");
    puts("  setcap cap_net_bind_service=+eip bindcap");
    puts("as root before using this.");
    return 1;
  }
  check(cap_free(caps) == 0);
  int r;
  r = prctl(PR_CAP_AMBIENT, PR_CAP_AMBIENT_RAISE, CAP_NET_BIND_SERVICE, 0, 0);
  check(r == 0);

  // run the subcommand.
  execvp(argv[1], argv + 1);
  printf("error running cmd: %m\n");
  return 1;
}

