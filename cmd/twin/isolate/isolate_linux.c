// libtwinisolate for Linux keeps a twin from sharing state with its original
// through the channels HOME and XDG redirection leave open. Every twin's
// launcher loads it with LD_PRELOAD; it's a no-op without TWIN_ROOT.
//
//   /tmp, /var/tmp        Fixed paths there move under TWIN_ROOT, a per-twin
//                         folder. X11's /tmp/.X* sockets and locks, and the
//                         D-Bus session buses some desktops keep in
//                         /tmp/dbus-*, stay shared.
//   $XDG_RUNTIME_DIR      App sockets there (single-instance locks, Discord's
//                         IPC…) move to a per-twin subfolder. Desktop services
//                         (Wayland, D-Bus, PulseAudio, PipeWire…) stay shared,
//                         or the twin couldn't draw or play sound.
//   machine identity      /etc/machine-id, /var/lib/dbus/machine-id, MAC
//                         addresses (getifaddrs, /sys/class/net/*/address) are
//                         mixed with a per-twin salt. TWIN_HARDWARE=shared
//                         turns this off.
//
// Environment: TWIN_ROOT, TWIN_NAME, TWIN_HARDWARE.
//
// Build: gcc -shared -fPIC -O2 -Wall -o libtwinisolate.so isolate_linux.c -ldl

#define _GNU_SOURCE
#include <dirent.h>
#include <dlfcn.h>
#include <fcntl.h>
#include <ifaddrs.h>
#include <limits.h>
#include <netpacket/packet.h>
#include <stdarg.h>
#include <stddef.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/un.h>
#include <unistd.h>

static int active, hwActive;
static char root[512], runtimeFrom[512], runtimeTo[640], self_[PATH_MAX];
static uint8_t salt[32];

#define REAL(name)                                              \
    static __typeof__(name) *real_##name;                       \
    if (!real_##name) real_##name = dlsym(RTLD_NEXT, #name)

static int under(const char *path, const char *dir) {
    size_t n = strlen(dir);
    return n && strncmp(path, dir, n) == 0 && (path[n] == '/' || path[n] == '\0');
}

// Desktop services in $XDG_RUNTIME_DIR that every app must share.
static const char *runtimeShared[] = {
    "bus", "wayland-", "pulse", "pipewire-", "dbus-1", "at-spi", "gnupg", "keyring", "systemd", "doc", "flatpak",
    ".flatpak", "app", "gvfs", "dconf", "gcr", "p11-kit", "ssh-agent", "openssh_agent", "gdm", "speech-dispatcher",
    "kwallet", "ICEauthority", "xauth_", ".mutter-Xwaylandauth", "snap.", "podman", "containers", "libvirt",
    "docker", "krb5cc", "wireplumber", "ibus", "fcitx", "kdeconnect", "Xwayland", "orbit-", NULL};

static int runtimeIsShared(const char *name) {
    for (const char **s = runtimeShared; *s; s++)
        if (strncmp(name, *s, strlen(*s)) == 0) return 1;
    return 0;
}

// remap returns path rewritten into buf, or path itself when no rule applies.
// noipa keeps its NULL check: glibc declares paths such as statx's as never
// NULL, so a copy inlined into the hooks would drop it, and Rust programs,
// which probe statx with a NULL path, would crash.
__attribute__((noipa)) static const char *remap(const char *path, char *buf, size_t n) {
    if (!active || !path || path[0] != '/') return path;
    if (under(path, root) || (*self_ && under(path, self_))) return path;
    if (strncmp(path, "/tmp/.X", 7) == 0 || strncmp(path, "/tmp/.ICE-unix", 14) == 0 ||
        strncmp(path, "/tmp/dbus-", 10) == 0)
        return path;

    if (under(path, "/tmp")) {
        snprintf(buf, n, "%s%s", root, path + 4);
        return buf;
    }
    if (under(path, "/var/tmp")) {
        snprintf(buf, n, "%s/var-tmp%s", root, path + 8);
        return buf;
    }
    if (*runtimeFrom && under(path, runtimeFrom) && !under(path, runtimeTo)) {
        const char *rest = path + strlen(runtimeFrom);
        if (*rest == '/' && rest[1] && !runtimeIsShared(rest + 1)) {
            snprintf(buf, n, "%s%s", runtimeTo, rest);
            return buf;
        }
    }
    if (hwActive) {
        if (!strcmp(path, "/etc/machine-id") || !strcmp(path, "/var/lib/dbus/machine-id")) {
            snprintf(buf, n, "%s/hw/machine-id", root);
            return buf;
        }
        if (strncmp(path, "/sys/class/net/", 15) == 0) {
            const char *slash = strchr(path + 15, '/');
            if (slash && !strcmp(slash, "/address")) {
                snprintf(buf, n, "%s/hw/net/%.*s", root, (int)(slash - path - 15), path + 15);
                return buf;
            }
        }
    }
    return path;
}

#define P(p, b) remap(p, b, sizeof b)

// ── file calls ─────────────────────────────────────────────────────────────

#define MODE_ARG(flags)                                                        \
    mode_t mode = 0;                                                           \
    if ((flags) & (O_CREAT | O_TMPFILE)) {                                     \
        va_list ap;                                                            \
        va_start(ap, flags);                                                   \
        mode = va_arg(ap, int);                                                \
        va_end(ap);                                                            \
    }

int open(const char *p, int flags, ...) { MODE_ARG(flags); REAL(open); char b[PATH_MAX]; return real_open(P(p, b), flags, mode); }
int open64(const char *p, int flags, ...) { MODE_ARG(flags); REAL(open64); char b[PATH_MAX]; return real_open64(P(p, b), flags, mode); }
int openat(int fd, const char *p, int flags, ...) { MODE_ARG(flags); REAL(openat); char b[PATH_MAX]; return real_openat(fd, P(p, b), flags, mode); }
int openat64(int fd, const char *p, int flags, ...) { MODE_ARG(flags); REAL(openat64); char b[PATH_MAX]; return real_openat64(fd, P(p, b), flags, mode); }
int __open_2(const char *p, int flags) { REAL(__open_2); char b[PATH_MAX]; return real___open_2(P(p, b), flags); }
int __open64_2(const char *p, int flags) { REAL(__open64_2); char b[PATH_MAX]; return real___open64_2(P(p, b), flags); }
int __openat_2(int fd, const char *p, int flags) { REAL(__openat_2); char b[PATH_MAX]; return real___openat_2(fd, P(p, b), flags); }
int creat(const char *p, mode_t m) { REAL(creat); char b[PATH_MAX]; return real_creat(P(p, b), m); }
FILE *fopen(const char *p, const char *m) { REAL(fopen); char b[PATH_MAX]; return real_fopen(P(p, b), m); }
FILE *fopen64(const char *p, const char *m) { REAL(fopen64); char b[PATH_MAX]; return real_fopen64(P(p, b), m); }
FILE *freopen(const char *p, const char *m, FILE *f) { REAL(freopen); char b[PATH_MAX]; return real_freopen(P(p, b), m, f); }
DIR *opendir(const char *p) { REAL(opendir); char b[PATH_MAX]; return real_opendir(P(p, b)); }
int stat(const char *p, struct stat *s) { REAL(stat); char b[PATH_MAX]; return real_stat(P(p, b), s); }
int lstat(const char *p, struct stat *s) { REAL(lstat); char b[PATH_MAX]; return real_lstat(P(p, b), s); }
int stat64(const char *p, struct stat64 *s) { REAL(stat64); char b[PATH_MAX]; return real_stat64(P(p, b), s); }
int lstat64(const char *p, struct stat64 *s) { REAL(lstat64); char b[PATH_MAX]; return real_lstat64(P(p, b), s); }
int fstatat(int fd, const char *p, struct stat *s, int f) { REAL(fstatat); char b[PATH_MAX]; return real_fstatat(fd, P(p, b), s, f); }
int fstatat64(int fd, const char *p, struct stat64 *s, int f) { REAL(fstatat64); char b[PATH_MAX]; return real_fstatat64(fd, P(p, b), s, f); }
int statx(int fd, const char *p, int f, unsigned int m, struct statx *s) { REAL(statx); char b[PATH_MAX]; return real_statx(fd, P(p, b), f, m, s); }
// Older glibc routes stat calls through these.
int __xstat(int v, const char *p, struct stat *s) { REAL(__xstat); char b[PATH_MAX]; return real___xstat(v, P(p, b), s); }
int __lxstat(int v, const char *p, struct stat *s) { REAL(__lxstat); char b[PATH_MAX]; return real___lxstat(v, P(p, b), s); }
int __xstat64(int v, const char *p, struct stat64 *s) { REAL(__xstat64); char b[PATH_MAX]; return real___xstat64(v, P(p, b), s); }
int __lxstat64(int v, const char *p, struct stat64 *s) { REAL(__lxstat64); char b[PATH_MAX]; return real___lxstat64(v, P(p, b), s); }
int __fxstatat(int v, int fd, const char *p, struct stat *s, int f) { REAL(__fxstatat); char b[PATH_MAX]; return real___fxstatat(v, fd, P(p, b), s, f); }
int access(const char *p, int m) { REAL(access); char b[PATH_MAX]; return real_access(P(p, b), m); }
int faccessat(int fd, const char *p, int m, int f) { REAL(faccessat); char b[PATH_MAX]; return real_faccessat(fd, P(p, b), m, f); }
int mkdir(const char *p, mode_t m) { REAL(mkdir); char b[PATH_MAX]; return real_mkdir(P(p, b), m); }
int mkdirat(int fd, const char *p, mode_t m) { REAL(mkdirat); char b[PATH_MAX]; return real_mkdirat(fd, P(p, b), m); }
int rmdir(const char *p) { REAL(rmdir); char b[PATH_MAX]; return real_rmdir(P(p, b)); }
int unlink(const char *p) { REAL(unlink); char b[PATH_MAX]; return real_unlink(P(p, b)); }
int unlinkat(int fd, const char *p, int f) { REAL(unlinkat); char b[PATH_MAX]; return real_unlinkat(fd, P(p, b), f); }
int chmod(const char *p, mode_t m) { REAL(chmod); char b[PATH_MAX]; return real_chmod(P(p, b), m); }
int chown(const char *p, uid_t u, gid_t g) { REAL(chown); char b[PATH_MAX]; return real_chown(P(p, b), u, g); }
int lchown(const char *p, uid_t u, gid_t g) { REAL(lchown); char b[PATH_MAX]; return real_lchown(P(p, b), u, g); }
ssize_t readlink(const char *p, char *o, size_t n) { REAL(readlink); char b[PATH_MAX]; return real_readlink(P(p, b), o, n); }
int symlink(const char *t, const char *p) { REAL(symlink); char b[PATH_MAX]; return real_symlink(t, P(p, b)); }
int link(const char *a, const char *z) { REAL(link); char b1[PATH_MAX], b2[PATH_MAX]; return real_link(P(a, b1), P(z, b2)); }
int rename(const char *a, const char *z) { REAL(rename); char b1[PATH_MAX], b2[PATH_MAX]; return real_rename(P(a, b1), P(z, b2)); }
int renameat(int fa, const char *a, int fz, const char *z) { REAL(renameat); char b1[PATH_MAX], b2[PATH_MAX]; return real_renameat(fa, P(a, b1), fz, P(z, b2)); }
int truncate(const char *p, off_t l) { REAL(truncate); char b[PATH_MAX]; return real_truncate(P(p, b), l); }
int mkfifo(const char *p, mode_t m) { REAL(mkfifo); char b[PATH_MAX]; return real_mkfifo(P(p, b), m); }
char *realpath(const char *p, char *o) { REAL(realpath); char b[PATH_MAX]; return real_realpath(P(p, b), o); }

// ── Unix sockets ───────────────────────────────────────────────────────────

// fixUnix rewrites a pathname socket address; it returns 0 when unchanged.
// Abstract sockets (leading NUL) have no path and stay as they are.
static int fixUnix(const struct sockaddr *a, socklen_t len, struct sockaddr_un *out, socklen_t *outlen) {
    if (!active || !a || a->sa_family != AF_UNIX || len <= offsetof(struct sockaddr_un, sun_path)) return 0;
    const struct sockaddr_un *u = (const struct sockaddr_un *)a;
    if (u->sun_path[0] == '\0') return 0;
    char path[sizeof u->sun_path + 1], b[PATH_MAX];
    size_t pl = len - offsetof(struct sockaddr_un, sun_path);
    if (pl > sizeof u->sun_path) pl = sizeof u->sun_path;
    memcpy(path, u->sun_path, pl);
    path[pl] = '\0';
    const char *r = remap(path, b, sizeof b);
    if (r == path || strlen(r) >= sizeof out->sun_path) return 0;
    memset(out, 0, sizeof *out);
    out->sun_family = AF_UNIX;
    strcpy(out->sun_path, r);
    *outlen = offsetof(struct sockaddr_un, sun_path) + strlen(r) + 1;
    return 1;
}

int bind(int s, const struct sockaddr *a, socklen_t l) {
    REAL(bind);
    struct sockaddr_un u;
    socklen_t ul;
    return fixUnix(a, l, &u, &ul) ? real_bind(s, (struct sockaddr *)&u, ul) : real_bind(s, a, l);
}

int connect(int s, const struct sockaddr *a, socklen_t l) {
    REAL(connect);
    struct sockaddr_un u;
    socklen_t ul;
    return fixUnix(a, l, &u, &ul) ? real_connect(s, (struct sockaddr *)&u, ul) : real_connect(s, a, l);
}

// ── machine identity ───────────────────────────────────────────────────────

static void mixMAC(uint8_t *mac) {
    int zero = 1;
    for (int i = 0; i < 6; i++) zero &= mac[i] == 0;
    if (zero) return;
    for (int i = 0; i < 6; i++) mac[i] ^= salt[i];
    mac[0] = (mac[0] | 0x02) & 0xFE; // locally administered, unicast
}

int getifaddrs(struct ifaddrs **list) {
    REAL(getifaddrs);
    int r = real_getifaddrs(list);
    if (r != 0 || !hwActive) return r;
    for (struct ifaddrs *i = *list; i; i = i->ifa_next) {
        if (!i->ifa_addr || i->ifa_addr->sa_family != AF_PACKET) continue;
        struct sockaddr_ll *ll = (struct sockaddr_ll *)i->ifa_addr;
        if (ll->sll_halen == 6) mixMAC(ll->sll_addr);
    }
    return r;
}

// writeFile writes s to path with the real calls (remapping is still off).
static void writeFile(const char *path, const char *s) {
    REAL(open);
    int fd = real_open(path, O_WRONLY | O_CREAT | O_TRUNC, 0600);
    if (fd < 0) return;
    ssize_t ignored = write(fd, s, strlen(s));
    (void)ignored;
    close(fd);
}

static void mkdirs(const char *dir) {
    REAL(mkdir);
    char b[PATH_MAX];
    snprintf(b, sizeof b, "%s", dir);
    for (char *p = b + 1; *p; p++) {
        if (*p != '/') continue;
        *p = '\0';
        real_mkdir(b, 0700);
        *p = '/';
    }
    real_mkdir(b, 0700);
}

// prepareHardware writes the twin's machine-id and MAC address files.
static void prepareHardware(void) {
    REAL(fopen);
    char path[PATH_MAX], line[128];
    snprintf(path, sizeof path, "%s/hw/net", root);
    mkdirs(path);

    FILE *f = real_fopen("/etc/machine-id", "r");
    if (!f) f = real_fopen("/var/lib/dbus/machine-id", "r");
    if (f && fgets(line, sizeof line, f)) {
        for (int i = 0; i < 32 && line[i]; i++) {
            int v = (line[i] >= 'a' ? line[i] - 'a' + 10 : line[i] - '0') ^ (salt[i % 32] & 0xF);
            line[i] = "0123456789abcdef"[v & 0xF];
        }
        snprintf(path, sizeof path, "%s/hw/machine-id", root);
        writeFile(path, line);
    }
    if (f) fclose(f);

    REAL(opendir);
    DIR *d = real_opendir("/sys/class/net");
    if (!d) return;
    for (struct dirent *e; (e = readdir(d));) {
        if (e->d_name[0] == '.') continue;
        snprintf(path, sizeof path, "/sys/class/net/%s/address", e->d_name);
        FILE *a = real_fopen(path, "r");
        unsigned m[6];
        if (a && fscanf(a, "%x:%x:%x:%x:%x:%x", &m[0], &m[1], &m[2], &m[3], &m[4], &m[5]) == 6) {
            uint8_t mac[6];
            for (int i = 0; i < 6; i++) mac[i] = (uint8_t)m[i];
            mixMAC(mac);
            snprintf(line, sizeof line, "%02x:%02x:%02x:%02x:%02x:%02x\n", mac[0], mac[1], mac[2], mac[3], mac[4], mac[5]);
            snprintf(path, sizeof path, "%s/hw/net/%s", root, e->d_name);
            writeFile(path, line);
        }
        if (a) fclose(a);
    }
    closedir(d);
}

// ── setup ──────────────────────────────────────────────────────────────────

__attribute__((constructor)) static void init(void) {
    const char *r = getenv("TWIN_ROOT"), *name = getenv("TWIN_NAME");
    if (!r || !*r || !name) return;
    snprintf(root, sizeof root, "%s", r);
    mkdirs(root);

    // A stable per-twin salt: FNV-1a over the name, stretched to 32 bytes.
    uint64_t h = 1469598103934665603ULL;
    for (int i = 0; i < 32; i++) {
        for (const char *c = name; *c; c++) h = (h ^ (uint8_t)*c) * 1099511628211ULL;
        h = (h ^ (uint64_t)i) * 1099511628211ULL;
        salt[i] = (uint8_t)(h >> 32);
    }

    const char *run = getenv("XDG_RUNTIME_DIR");
    if (run && *run) {
        snprintf(runtimeFrom, sizeof runtimeFrom, "%s", run);
        snprintf(runtimeTo, sizeof runtimeTo, "%s/twin.%s", run, strrchr(root, '.') ? strrchr(root, '.') + 1 : "twin");
        mkdirs(runtimeTo);
    }

    char exe[PATH_MAX];
    ssize_t n = readlink("/proc/self/exe", exe, sizeof exe - 1);
    if (n > 0) {
        exe[n] = '\0';
        *strrchr(exe, '/') = '\0';
        snprintf(self_, sizeof self_, "%s", exe);
    }

    const char *hw = getenv("TWIN_HARDWARE");
    hwActive = !(hw && !strcmp(hw, "shared"));
    if (hwActive) prepareHardware();
    active = 1;
}
