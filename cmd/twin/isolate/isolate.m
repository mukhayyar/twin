// libtwinisolate keeps a twin from sharing state with its original through
// the channels HOME redirection alone leaves open. Every twin's launcher
// loads it with DYLD_INSERT_LIBRARIES; it's a no-op without TWIN_ROOT.
//
//   /tmp, /private/tmp    Fixed paths there (RustDesk serves its settings over
//                         /tmp/RustDesk-<uid>/ipc) move under TWIN_ROOT, a
//                         per-twin folder. launchd's com.apple.* sockets
//                         (ssh-agent and friends) are left alone.
//   temp & cache dirs     confstr(_CS_DARWIN_USER_TEMP_DIR / CACHE_DIR), which
//                         NSTemporaryDirectory() uses, point into the twin.
//   preferences           cfprefsd keeps every domain in the real
//                         ~/Library/Preferences, so explicitly named domains
//                         (suites, the original's bundle ID) are renamed:
//                         the original's ID becomes the twin's, anything else
//                         gets ".twin-<label>". Apple domains stay shared.
//   keychain              Item services (and internet-password servers) get
//                         " [twin <name>]" appended, so twins never read or
//                         overwrite the original's tokens and keys.
//   machine identity      MAC addresses (getifaddrs, IOKit), the hardware UUID
//                         (gethostuuid, IOPlatformUUID, kern.uuid) and serial
//                         number are mixed with a per-twin salt, so apps that
//                         derive an ID from the hardware (RustDesk does, from
//                         the MAC) see each twin as its own machine. Stable:
//                         the same twin always gets the same values.
//                         TWIN_HARDWARE=shared turns this part off, for apps
//                         licensed per device.
//
// Twins are launched by macOS with their Info.plist's LSEnvironment, which
// can't set HOME (launchd overrides it), so TWIN_HOME carries the profile and
// this library moves HOME there before the app runs, after linking the login
// keychain into it (macOS looks for the keychain under $HOME).
//
// Environment: TWIN_HOME, TWIN_ROOT, TWIN_CACHE, TWIN_NAME, TWIN_LABEL,
// TWIN_ID (the twin's bundle ID), TWIN_ORIG_ID, TWIN_HARDWARE, and optional
// TWIN_REMAP extra prefix pairs ("from=to", separated by ':').
//
// Build: clang -arch arm64 -arch x86_64 -mmacosx-version-min=11.0 -dynamiclib -O2 -fobjc-arc \
//          -framework Foundation -framework Security -framework IOKit -o libtwinisolate.dylib isolate.m

#import <CommonCrypto/CommonDigest.h>
#import <Foundation/Foundation.h>
#import <IOKit/IOKitLib.h>
#import <Security/Security.h>
#include <dlfcn.h>
#include <fcntl.h>
#include <ifaddrs.h>
#include <net/if_dl.h>
#include <sys/sysctl.h>
#include <uuid/uuid.h>
#include <mach-o/dyld.h>
#include <pwd.h>
#include <objc/runtime.h>
#include <stdarg.h>
#include <sys/attr.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/un.h>
#include <unistd.h>

#pragma clang diagnostic ignored "-Wdeprecated-declarations"

#define INTERPOSE(replacement, original)                                                     \
    __attribute__((used)) static const struct { const void *r, *o; } interpose_##original \
        __attribute__((section("__DATA,__interpose"))) = {(const void *)&replacement, (const void *)&original}

static int active, hwActive; // hwActive: give the twin its own hardware IDs
static char root[512], tempDir[600], cacheDir[1024];
static CFStringRef twinID, origID, label, keySuffix;

// ── paths ──────────────────────────────────────────────────────────────────

#define MAX_PAIRS 12
static struct { char from[512], to[600]; size_t len; } pairs[MAX_PAIRS];
static int npairs;

static void addPair(const char *from, const char *to) {
    if (npairs == MAX_PAIRS || !*from) return;
    strlcpy(pairs[npairs].from, from, sizeof pairs[npairs].from);
    strlcpy(pairs[npairs].to, to, sizeof pairs[npairs].to);
    pairs[npairs].len = strlen(pairs[npairs].from);
    npairs++;
}

static int hasPrefixPath(const char *path, const char *prefix, size_t len) {
    return strncmp(path, prefix, len) == 0 && (path[len] == '/' || path[len] == '\0');
}

// self_ is the twin's own bundle (or executable folder). It's never remapped,
// so an app that happens to live in /tmp can still read itself.
static char self_[1024];

// remap returns path rewritten into buf, or path itself when no rule applies.
static const char *remap(const char *path, char *buf, size_t n) {
    if (!active || !path || path[0] != '/') return path;
    if (hasPrefixPath(path, root, strlen(root))) return path; // already the twin's
    if (*self_ && hasPrefixPath(path, self_, strlen(self_))) return path;
    for (int i = 0; i < npairs; i++) {
        if (!hasPrefixPath(path, pairs[i].from, pairs[i].len)) continue;
        const char *rest = path + pairs[i].len;
        if (strncmp(rest, "/com.apple.", 11) == 0) return path; // launchd sockets
        snprintf(buf, n, "%s%s", pairs[i].to, rest);
        return buf;
    }
    return path;
}

#define P(p, b) remap(p, b, sizeof b)

static int t_open(const char *p, int flags, ...) {
    char b[1024];
    int mode = 0;
    if (flags & O_CREAT) {
        va_list ap;
        va_start(ap, flags);
        mode = va_arg(ap, int);
        va_end(ap);
    }
    return open(P(p, b), flags, mode);
}
INTERPOSE(t_open, open);

static int t_openat(int fd, const char *p, int flags, ...) {
    char b[1024];
    int mode = 0;
    if (flags & O_CREAT) {
        va_list ap;
        va_start(ap, flags);
        mode = va_arg(ap, int);
        va_end(ap);
    }
    return openat(fd, P(p, b), flags, mode);
}
INTERPOSE(t_openat, openat);

static int t_mkdir(const char *p, mode_t m) { char b[1024]; return mkdir(P(p, b), m); }
INTERPOSE(t_mkdir, mkdir);
static int t_mkdirat(int fd, const char *p, mode_t m) { char b[1024]; return mkdirat(fd, P(p, b), m); }
INTERPOSE(t_mkdirat, mkdirat);
static int t_rmdir(const char *p) { char b[1024]; return rmdir(P(p, b)); }
INTERPOSE(t_rmdir, rmdir);
static int t_unlink(const char *p) { char b[1024]; return unlink(P(p, b)); }
INTERPOSE(t_unlink, unlink);
static int t_unlinkat(int fd, const char *p, int f) { char b[1024]; return unlinkat(fd, P(p, b), f); }
INTERPOSE(t_unlinkat, unlinkat);
static int t_chmod(const char *p, mode_t m) { char b[1024]; return chmod(P(p, b), m); }
INTERPOSE(t_chmod, chmod);
static int t_chown(const char *p, uid_t u, gid_t g) { char b[1024]; return chown(P(p, b), u, g); }
INTERPOSE(t_chown, chown);
static int t_access(const char *p, int m) { char b[1024]; return access(P(p, b), m); }
INTERPOSE(t_access, access);
static int t_faccessat(int fd, const char *p, int m, int f) { char b[1024]; return faccessat(fd, P(p, b), m, f); }
INTERPOSE(t_faccessat, faccessat);
static int t_stat(const char *p, struct stat *s) { char b[1024]; return stat(P(p, b), s); }
INTERPOSE(t_stat, stat);
static int t_lstat(const char *p, struct stat *s) { char b[1024]; return lstat(P(p, b), s); }
INTERPOSE(t_lstat, lstat);
static int t_fstatat(int fd, const char *p, struct stat *s, int f) { char b[1024]; return fstatat(fd, P(p, b), s, f); }
INTERPOSE(t_fstatat, fstatat);
static int t_truncate(const char *p, off_t l) { char b[1024]; return truncate(P(p, b), l); }
INTERPOSE(t_truncate, truncate);
static int t_chdir(const char *p) { char b[1024]; return chdir(P(p, b)); }
INTERPOSE(t_chdir, chdir);
static ssize_t t_readlink(const char *p, char *o, size_t n) { char b[1024]; return readlink(P(p, b), o, n); }
INTERPOSE(t_readlink, readlink);
static int t_mkfifo(const char *p, mode_t m) { char b[1024]; return mkfifo(P(p, b), m); }
INTERPOSE(t_mkfifo, mkfifo);
static int t_symlink(const char *target, const char *p) { char b[1024]; return symlink(target, P(p, b)); }
INTERPOSE(t_symlink, symlink);
static int t_rename(const char *a, const char *z) { char b1[1024], b2[1024]; return rename(P(a, b1), P(z, b2)); }
INTERPOSE(t_rename, rename);
static int t_renameat(int fa, const char *a, int fz, const char *z) {
    char b1[1024], b2[1024];
    return renameat(fa, P(a, b1), fz, P(z, b2));
}
INTERPOSE(t_renameat, renameat);
static int t_getattrlist(const char *p, void *al, void *buf, size_t n, unsigned int o) {
    char b[1024];
    return getattrlist(P(p, b), al, buf, n, o);
}
INTERPOSE(t_getattrlist, getattrlist);
static int t_setattrlist(const char *p, void *al, void *buf, size_t n, unsigned int o) {
    char b[1024];
    return setattrlist(P(p, b), al, buf, n, o);
}
INTERPOSE(t_setattrlist, setattrlist);

// fixUnix rewrites a Unix-socket address; it returns 0 when nothing changed.
static int fixUnix(const struct sockaddr *addr, struct sockaddr_un *out, socklen_t *outlen) {
    if (!active || !addr || addr->sa_family != AF_UNIX) return 0;
    const struct sockaddr_un *u = (const struct sockaddr_un *)addr;
    char path[sizeof u->sun_path + 1], b[1024];
    memcpy(path, u->sun_path, sizeof u->sun_path);
    path[sizeof u->sun_path] = '\0';
    const char *r = remap(path, b, sizeof b);
    if (r == path || strlen(r) >= sizeof out->sun_path) return 0;
    memset(out, 0, sizeof *out);
    out->sun_family = AF_UNIX;
    strlcpy(out->sun_path, r, sizeof out->sun_path);
    out->sun_len = SUN_LEN(out);
    *outlen = out->sun_len;
    return 1;
}

static int t_bind(int s, const struct sockaddr *a, socklen_t l) {
    struct sockaddr_un u;
    socklen_t ul;
    return fixUnix(a, &u, &ul) ? bind(s, (struct sockaddr *)&u, ul) : bind(s, a, l);
}
INTERPOSE(t_bind, bind);

static int t_connect(int s, const struct sockaddr *a, socklen_t l) {
    struct sockaddr_un u;
    socklen_t ul;
    return fixUnix(a, &u, &ul) ? connect(s, (struct sockaddr *)&u, ul) : connect(s, a, l);
}
INTERPOSE(t_connect, connect);

// ── temp & cache dirs ──────────────────────────────────────────────────────

static size_t t_confstr(int name, char *buf, size_t len) {
    const char *v = !active ? NULL
                  : name == _CS_DARWIN_USER_TEMP_DIR ? tempDir
                  : name == _CS_DARWIN_USER_CACHE_DIR ? cacheDir : NULL;
    if (!v) return confstr(name, buf, len);
    if (buf && len) strlcpy(buf, v, len);
    return strlen(v) + 1;
}
INTERPOSE(t_confstr, confstr);

// ── preferences ────────────────────────────────────────────────────────────

// mapDomain returns a retained preferences domain to use instead of d.
static CFStringRef mapDomain(CFStringRef d) {
    if (!active || !d || CFGetTypeID(d) != CFStringGetTypeID() ||
        CFEqual(d, kCFPreferencesCurrentApplication) || CFEqual(d, kCFPreferencesAnyApplication) ||
        CFEqual(d, CFSTR(".GlobalPreferences")) || CFEqual(d, CFSTR("NSGlobalDomain")) ||
        CFStringHasPrefix(d, CFSTR("com.apple.")) || (twinID && CFEqual(d, twinID)))
        return d ? CFRetain(d) : NULL;
    if (origID && CFStringCompare(d, origID, kCFCompareCaseInsensitive) == kCFCompareEqualTo)
        return CFRetain(twinID);
    return CFStringCreateWithFormat(NULL, NULL, CFSTR("%@.twin-%@"), d, label);
}

#define WITH_DOMAIN(app, expr)          \
    ({                                  \
        CFStringRef m_ = mapDomain(app); \
        __auto_type r_ = (expr);        \
        if (m_) CFRelease(m_);          \
        r_;                             \
    })

static CFPropertyListRef t_CFPreferencesCopyAppValue(CFStringRef k, CFStringRef a) {
    return WITH_DOMAIN(a, CFPreferencesCopyAppValue(k, m_));
}
INTERPOSE(t_CFPreferencesCopyAppValue, CFPreferencesCopyAppValue);
static void t_CFPreferencesSetAppValue(CFStringRef k, CFPropertyListRef v, CFStringRef a) {
    CFStringRef m = mapDomain(a);
    CFPreferencesSetAppValue(k, v, m);
    if (m) CFRelease(m);
}
INTERPOSE(t_CFPreferencesSetAppValue, CFPreferencesSetAppValue);
static Boolean t_CFPreferencesAppSynchronize(CFStringRef a) { return WITH_DOMAIN(a, CFPreferencesAppSynchronize(m_)); }
INTERPOSE(t_CFPreferencesAppSynchronize, CFPreferencesAppSynchronize);
static Boolean t_CFPreferencesGetAppBooleanValue(CFStringRef k, CFStringRef a, Boolean *ok) {
    return WITH_DOMAIN(a, CFPreferencesGetAppBooleanValue(k, m_, ok));
}
INTERPOSE(t_CFPreferencesGetAppBooleanValue, CFPreferencesGetAppBooleanValue);
static CFIndex t_CFPreferencesGetAppIntegerValue(CFStringRef k, CFStringRef a, Boolean *ok) {
    return WITH_DOMAIN(a, CFPreferencesGetAppIntegerValue(k, m_, ok));
}
INTERPOSE(t_CFPreferencesGetAppIntegerValue, CFPreferencesGetAppIntegerValue);
static CFPropertyListRef t_CFPreferencesCopyValue(CFStringRef k, CFStringRef a, CFStringRef u, CFStringRef h) {
    return WITH_DOMAIN(a, CFPreferencesCopyValue(k, m_, u, h));
}
INTERPOSE(t_CFPreferencesCopyValue, CFPreferencesCopyValue);
static void t_CFPreferencesSetValue(CFStringRef k, CFPropertyListRef v, CFStringRef a, CFStringRef u, CFStringRef h) {
    CFStringRef m = mapDomain(a);
    CFPreferencesSetValue(k, v, m, u, h);
    if (m) CFRelease(m);
}
INTERPOSE(t_CFPreferencesSetValue, CFPreferencesSetValue);
static Boolean t_CFPreferencesSynchronize(CFStringRef a, CFStringRef u, CFStringRef h) {
    return WITH_DOMAIN(a, CFPreferencesSynchronize(m_, u, h));
}
INTERPOSE(t_CFPreferencesSynchronize, CFPreferencesSynchronize);
static CFArrayRef t_CFPreferencesCopyKeyList(CFStringRef a, CFStringRef u, CFStringRef h) {
    return WITH_DOMAIN(a, CFPreferencesCopyKeyList(m_, u, h));
}
INTERPOSE(t_CFPreferencesCopyKeyList, CFPreferencesCopyKeyList);
static CFDictionaryRef t_CFPreferencesCopyMultiple(CFArrayRef ks, CFStringRef a, CFStringRef u, CFStringRef h) {
    return WITH_DOMAIN(a, CFPreferencesCopyMultiple(ks, m_, u, h));
}
INTERPOSE(t_CFPreferencesCopyMultiple, CFPreferencesCopyMultiple);
static void t_CFPreferencesSetMultiple(CFDictionaryRef s, CFArrayRef r, CFStringRef a, CFStringRef u, CFStringRef h) {
    CFStringRef m = mapDomain(a);
    CFPreferencesSetMultiple(s, r, m, u, h);
    if (m) CFRelease(m);
}
INTERPOSE(t_CFPreferencesSetMultiple, CFPreferencesSetMultiple);
static void t_CFPreferencesAddSuitePreferencesToApp(CFStringRef a, CFStringRef s) {
    CFStringRef m = mapDomain(s);
    CFPreferencesAddSuitePreferencesToApp(a, m);
    if (m) CFRelease(m);
}
INTERPOSE(t_CFPreferencesAddSuitePreferencesToApp, CFPreferencesAddSuitePreferencesToApp);

// NSUserDefaults talks to cfprefsd through private CoreFoundation code, so its
// named-domain entry points are wrapped at the Objective-C level instead.
static NSString *mapSuite(NSString *s) {
    if (!s) return s;
    return (__bridge_transfer NSString *)mapDomain((__bridge CFStringRef)s);
}

static void swizzle(Class c, SEL sel, IMP replacement, IMP *original) {
    Method m = class_getInstanceMethod(c, sel);
    if (m) *original = method_setImplementation(m, replacement);
}

static IMP origInitSuite, origAddSuite, origRemoveSuite, origPersistent, origSetPersistent, origRemovePersistent;

static id twinInitSuite(id self, SEL _cmd, NSString *s) {
    return ((id (*)(id, SEL, NSString *))origInitSuite)(self, _cmd, mapSuite(s));
}
static void twinAddSuite(id self, SEL _cmd, NSString *s) {
    ((void (*)(id, SEL, NSString *))origAddSuite)(self, _cmd, mapSuite(s));
}
static void twinRemoveSuite(id self, SEL _cmd, NSString *s) {
    ((void (*)(id, SEL, NSString *))origRemoveSuite)(self, _cmd, mapSuite(s));
}
static id twinPersistent(id self, SEL _cmd, NSString *s) {
    return ((id (*)(id, SEL, NSString *))origPersistent)(self, _cmd, mapSuite(s));
}
static void twinSetPersistent(id self, SEL _cmd, NSDictionary *d, NSString *s) {
    ((void (*)(id, SEL, NSDictionary *, NSString *))origSetPersistent)(self, _cmd, d, mapSuite(s));
}
static void twinRemovePersistent(id self, SEL _cmd, NSString *s) {
    ((void (*)(id, SEL, NSString *))origRemovePersistent)(self, _cmd, mapSuite(s));
}

// ── keychain ───────────────────────────────────────────────────────────────

static CFStringRef suffixed(CFStringRef s) {
    if (CFStringHasSuffix(s, keySuffix)) return CFRetain(s);
    return CFStringCreateWithFormat(NULL, NULL, CFSTR("%@%@"), s, keySuffix);
}

// scopeQuery returns a retained copy of d with its service/server namespaced.
static CFDictionaryRef scopeQuery(CFDictionaryRef d) {
    if (!active || !d) return d ? CFRetain(d) : NULL;
    CFMutableDictionaryRef m = CFDictionaryCreateMutableCopy(NULL, 0, d);
    const CFStringRef keys[] = {kSecAttrService, kSecAttrServer};
    for (int i = 0; i < 2; i++) {
        CFTypeRef v = CFDictionaryGetValue(d, keys[i]);
        if (v && CFGetTypeID(v) == CFStringGetTypeID()) {
            CFStringRef s = suffixed(v);
            CFDictionarySetValue(m, keys[i], s);
            CFRelease(s);
        }
    }
    return m;
}

// unscope strips the suffix from attributes handed back to the app.
static CFTypeRef unscope(CFTypeRef r) {
    if (!r) return r;
    if (CFGetTypeID(r) == CFArrayGetTypeID()) {
        CFIndex n = CFArrayGetCount(r);
        CFMutableArrayRef a = CFArrayCreateMutable(NULL, n, &kCFTypeArrayCallBacks);
        for (CFIndex i = 0; i < n; i++) {
            CFTypeRef e = unscope(CFRetain(CFArrayGetValueAtIndex(r, i)));
            CFArrayAppendValue(a, e);
            CFRelease(e);
        }
        CFRelease(r);
        return a;
    }
    if (CFGetTypeID(r) != CFDictionaryGetTypeID()) return r;
    CFMutableDictionaryRef m = CFDictionaryCreateMutableCopy(NULL, 0, r);
    const CFStringRef keys[] = {kSecAttrService, kSecAttrServer};
    for (int i = 0; i < 2; i++) {
        CFTypeRef v = CFDictionaryGetValue(m, keys[i]);
        if (v && CFGetTypeID(v) == CFStringGetTypeID() && CFStringHasSuffix(v, keySuffix)) {
            CFIndex len = CFStringGetLength(v) - CFStringGetLength(keySuffix);
            CFStringRef s = CFStringCreateWithSubstring(NULL, v, CFRangeMake(0, len));
            CFDictionarySetValue(m, keys[i], s);
            CFRelease(s);
        }
    }
    CFRelease(r);
    return m;
}

static OSStatus t_SecItemCopyMatching(CFDictionaryRef q, CFTypeRef *result) {
    CFDictionaryRef s = scopeQuery(q);
    OSStatus st = SecItemCopyMatching(s, result);
    if (s) CFRelease(s);
    if (active && st == errSecSuccess && result && *result) *result = unscope(*result);
    return st;
}
INTERPOSE(t_SecItemCopyMatching, SecItemCopyMatching);

static OSStatus t_SecItemAdd(CFDictionaryRef attrs, CFTypeRef *result) {
    CFDictionaryRef s = scopeQuery(attrs);
    OSStatus st = SecItemAdd(s, result);
    if (s) CFRelease(s);
    if (active && st == errSecSuccess && result && *result) *result = unscope(*result);
    return st;
}
INTERPOSE(t_SecItemAdd, SecItemAdd);

static OSStatus t_SecItemUpdate(CFDictionaryRef q, CFDictionaryRef attrs) {
    CFDictionaryRef s = scopeQuery(q), a = scopeQuery(attrs);
    OSStatus st = SecItemUpdate(s, a);
    if (s) CFRelease(s);
    if (a) CFRelease(a);
    return st;
}
INTERPOSE(t_SecItemUpdate, SecItemUpdate);

static OSStatus t_SecItemDelete(CFDictionaryRef q) {
    CFDictionaryRef s = scopeQuery(q);
    OSStatus st = SecItemDelete(s);
    if (s) CFRelease(s);
    return st;
}
INTERPOSE(t_SecItemDelete, SecItemDelete);

// scopedName appends the suffix to a (length, bytes) keychain name.
static const char *scopedName(UInt32 *len, const char *name, char *buf, size_t n) {
    if (!active || !name) return name;
    char suffix[256];
    CFStringGetCString(keySuffix, suffix, sizeof suffix, kCFStringEncodingUTF8);
    size_t sl = strlen(suffix);
    if (*len >= sl && memcmp(name + *len - sl, suffix, sl) == 0) return name;
    if (*len + sl + 1 > n) return name;
    memcpy(buf, name, *len);
    memcpy(buf + *len, suffix, sl + 1);
    *len += (UInt32)sl;
    return buf;
}

static OSStatus t_SecKeychainFindGenericPassword(CFTypeRef kc, UInt32 sl, const char *s, UInt32 al, const char *a,
                                                 UInt32 *pl, void **pd, SecKeychainItemRef *item) {
    char b[1024];
    s = scopedName(&sl, s, b, sizeof b);
    return SecKeychainFindGenericPassword(kc, sl, s, al, a, pl, pd, item);
}
INTERPOSE(t_SecKeychainFindGenericPassword, SecKeychainFindGenericPassword);

static OSStatus t_SecKeychainAddGenericPassword(SecKeychainRef kc, UInt32 sl, const char *s, UInt32 al, const char *a,
                                                UInt32 pl, const void *pd, SecKeychainItemRef *item) {
    char b[1024];
    s = scopedName(&sl, s, b, sizeof b);
    return SecKeychainAddGenericPassword(kc, sl, s, al, a, pl, pd, item);
}
INTERPOSE(t_SecKeychainAddGenericPassword, SecKeychainAddGenericPassword);

static OSStatus t_SecKeychainFindInternetPassword(CFTypeRef kc, UInt32 sl, const char *s, UInt32 dl, const char *d,
                                                  UInt32 al, const char *a, UInt32 pthl, const char *pth, UInt16 port,
                                                  SecProtocolType pr, SecAuthenticationType at, UInt32 *pl, void **pd,
                                                  SecKeychainItemRef *item) {
    char b[1024];
    s = scopedName(&sl, s, b, sizeof b);
    return SecKeychainFindInternetPassword(kc, sl, s, dl, d, al, a, pthl, pth, port, pr, at, pl, pd, item);
}
INTERPOSE(t_SecKeychainFindInternetPassword, SecKeychainFindInternetPassword);

static OSStatus t_SecKeychainAddInternetPassword(SecKeychainRef kc, UInt32 sl, const char *s, UInt32 dl, const char *d,
                                                 UInt32 al, const char *a, UInt32 pthl, const char *pth, UInt16 port,
                                                 SecProtocolType pr, SecAuthenticationType at, UInt32 pl, const void *pd,
                                                 SecKeychainItemRef *item) {
    char b[1024];
    s = scopedName(&sl, s, b, sizeof b);
    return SecKeychainAddInternetPassword(kc, sl, s, dl, d, al, a, pthl, pth, port, pr, at, pl, pd, item);
}
INTERPOSE(t_SecKeychainAddInternetPassword, SecKeychainAddInternetPassword);

// ── machine identity ───────────────────────────────────────────────────────

static uint8_t salt[32]; // SHA-256 of the twin's name

static void mixBytes(uint8_t *b, size_t n) {
    for (size_t i = 0; i < n; i++) b[i] ^= salt[i % sizeof salt];
}

// mixMAC changes a hardware address but keeps it a valid unicast,
// locally administered one.
static void mixMAC(uint8_t *mac) {
    int zero = 1;
    for (int i = 0; i < 6; i++) zero &= mac[i] == 0;
    if (zero) return;
    mixBytes(mac, 6);
    mac[0] = (mac[0] | 0x02) & 0xFE;
}

static int t_getifaddrs(struct ifaddrs **list) {
    int r = getifaddrs(list);
    if (r != 0 || !hwActive) return r;
    for (struct ifaddrs *i = *list; i; i = i->ifa_next) {
        if (!i->ifa_addr || i->ifa_addr->sa_family != AF_LINK) continue;
        struct sockaddr_dl *dl = (struct sockaddr_dl *)i->ifa_addr;
        if (dl->sdl_alen == 6) mixMAC((uint8_t *)LLADDR(dl));
    }
    return r;
}
INTERPOSE(t_getifaddrs, getifaddrs);

static int t_gethostuuid(uuid_t id, const struct timespec *wait) {
    int r = gethostuuid(id, wait);
    if (r == 0 && hwActive) mixBytes(id, sizeof(uuid_t));
    return r;
}
INTERPOSE(t_gethostuuid, gethostuuid);

// mixUUIDString returns a retained copy of a UUID string with the salt mixed in.
static CFStringRef mixUUIDString(CFStringRef s) {
    char str[64];
    uuid_t u;
    if (!CFStringGetCString(s, str, sizeof str, kCFStringEncodingUTF8) || uuid_parse(str, u) != 0) return CFRetain(s);
    mixBytes(u, sizeof u);
    uuid_unparse_upper(u, str);
    return CFStringCreateWithCString(NULL, str, kCFStringEncodingUTF8);
}

// mixSerial keeps a serial's length and character set (digits, uppercase letters).
static CFStringRef mixSerial(CFStringRef s) {
    char str[128];
    if (!CFStringGetCString(s, str, sizeof str, kCFStringEncodingUTF8)) return CFRetain(s);
    static const char abc[] = "0123456789ABCDEFGHJKLMNPQRSTUVWXYZ";
    for (size_t i = 0; str[i]; i++)
        if (isalnum((unsigned char)str[i])) str[i] = abc[(str[i] + salt[i % sizeof salt]) % (sizeof abc - 1)];
    return CFStringCreateWithCString(NULL, str, kCFStringEncodingUTF8);
}

static CFTypeRef mixProperty(CFStringRef key, CFTypeRef v) {
    if (!hwActive || !v || !key) return v;
    CFTypeRef out = v;
    if (CFGetTypeID(v) == CFStringGetTypeID() && CFEqual(key, CFSTR("IOPlatformUUID"))) out = mixUUIDString(v);
    else if (CFGetTypeID(v) == CFStringGetTypeID() && CFEqual(key, CFSTR("IOPlatformSerialNumber"))) out = mixSerial(v);
    else if (CFGetTypeID(v) == CFDataGetTypeID() && CFEqual(key, CFSTR("IOMACAddress")) && CFDataGetLength(v) == 6) {
        uint8_t mac[6];
        CFDataGetBytes(v, CFRangeMake(0, 6), mac);
        mixMAC(mac);
        out = CFDataCreate(NULL, mac, 6);
    }
    if (out != v) CFRelease(v);
    return out;
}

static CFTypeRef t_IORegistryEntryCreateCFProperty(io_registry_entry_t e, CFStringRef key, CFAllocatorRef a, IOOptionBits o) {
    return mixProperty(key, IORegistryEntryCreateCFProperty(e, key, a, o));
}
INTERPOSE(t_IORegistryEntryCreateCFProperty, IORegistryEntryCreateCFProperty);

static CFTypeRef t_IORegistryEntrySearchCFProperty(io_registry_entry_t e, const io_name_t plane, CFStringRef key,
                                                   CFAllocatorRef a, IOOptionBits o) {
    return mixProperty(key, IORegistryEntrySearchCFProperty(e, plane, key, a, o));
}
INTERPOSE(t_IORegistryEntrySearchCFProperty, IORegistryEntrySearchCFProperty);

static int t_sysctlbyname(const char *name, void *old, size_t *oldlen, void *new, size_t newlen) {
    int r = sysctlbyname(name, old, oldlen, new, newlen);
    if (r == 0 && hwActive && old && oldlen && name && strcmp(name, "kern.uuid") == 0) {
        uuid_t u;
        if (uuid_parse(old, u) == 0) {
            mixBytes(u, sizeof u);
            uuid_unparse_upper(u, old);
        }
    }
    return r;
}
INTERPOSE(t_sysctlbyname, sysctlbyname);

// ── setup ──────────────────────────────────────────────────────────────────

// mkdirs creates dir and any missing parents (it runs before remapping is on).
static void mkdirs(const char *dir) {
    char b[1024];
    strlcpy(b, dir, sizeof b);
    for (char *p = b + 1; *p; p++) {
        if (*p != '/') continue;
        *p = '\0';
        mkdir(b, 0700);
        *p = '/';
    }
    mkdir(b, 0700);
}

static CFStringRef envString(const char *name) {
    const char *v = getenv(name);
    return v && *v ? CFStringCreateWithCString(NULL, v, kCFStringEncodingUTF8) : NULL;
}

__attribute__((constructor)) static void init(void) {
    const char *r = getenv("TWIN_ROOT");
    const char *name = getenv("TWIN_NAME");
    if (!r || !*r || !name) return;

    strlcpy(root, r, sizeof root);
    snprintf(tempDir, sizeof tempDir, "%s/T/", root);
    const char *c = getenv("TWIN_CACHE");
    snprintf(cacheDir, sizeof cacheDir, "%s/", c && *c ? c : root);
    mkdirs(tempDir);
    mkdirs(cacheDir);

    // Move HOME into the profile, keeping the login keychain reachable.
    const char *home = getenv("TWIN_HOME");
    if (home && *home) {
        char dir[1024], link[1024], target[1024];
        snprintf(dir, sizeof dir, "%s/Library", home);
        mkdirs(dir);
        struct passwd *pw = getpwuid(getuid());
        snprintf(link, sizeof link, "%s/Library/Keychains", home);
        struct stat st;
        if (pw && lstat(link, &st) != 0) {
            snprintf(target, sizeof target, "%s/Library/Keychains", pw->pw_dir);
            symlink(target, link);
        }
        setenv("HOME", home, 1);
        setenv("CFFIXED_USER_HOME", home, 1);
    }
    setenv("TMPDIR", tempDir, 1);

    // Twins load this library as @executable_path/libtwinisolate.dylib, which
    // child processes (Electron and Chromium helper apps) would resolve against
    // their own folder and fail to launch. Hand them this copy's absolute path.
    Dl_info self;
    char lib[1024];
    if (dladdr((const void *)init, &self) && self.dli_fname && realpath(self.dli_fname, lib))
        setenv("DYLD_INSERT_LIBRARIES", lib, 1);

    char exe[1024], real[1024];
    uint32_t size = sizeof exe;
    if (_NSGetExecutablePath(exe, &size) == 0 && realpath(exe, real)) {
        char *app = strstr(real, ".app/Contents/MacOS/");
        if (app) app[4] = '\0';
        else *strrchr(real, '/') = '\0';
        strlcpy(self_, real, sizeof self_);
    }

    addPair("/private/tmp", root);
    addPair("/tmp", root);
    const char *extra = getenv("TWIN_REMAP");
    if (extra) {
        char *spec = strdup(extra), *save = NULL;
        for (char *tok = strtok_r(spec, ":", &save); tok; tok = strtok_r(NULL, ":", &save)) {
            char *eq = strchr(tok, '=');
            if (!eq) continue;
            *eq = '\0';
            addPair(tok, eq + 1);
        }
        free(spec);
    }

    twinID = envString("TWIN_ID");
    origID = envString("TWIN_ORIG_ID");
    label = envString("TWIN_LABEL");
    if (!label) label = CFSTR("twin");
    keySuffix = CFStringCreateWithFormat(NULL, NULL, CFSTR(" [twin %s]"), name);
    CC_SHA256(name, (CC_LONG)strlen(name), salt);
    active = 1;
    const char *hw = getenv("TWIN_HARDWARE");
    hwActive = !(hw && strcmp(hw, "shared") == 0);

    Class defaults = objc_getClass("NSUserDefaults");
    swizzle(defaults, @selector(initWithSuiteName:), (IMP)twinInitSuite, &origInitSuite);
    swizzle(defaults, @selector(addSuiteNamed:), (IMP)twinAddSuite, &origAddSuite);
    swizzle(defaults, @selector(removeSuiteNamed:), (IMP)twinRemoveSuite, &origRemoveSuite);
    swizzle(defaults, @selector(persistentDomainForName:), (IMP)twinPersistent, &origPersistent);
    swizzle(defaults, @selector(setPersistentDomain:forName:), (IMP)twinSetPersistent, &origSetPersistent);
    swizzle(defaults, @selector(removePersistentDomainForName:), (IMP)twinRemovePersistent, &origRemovePersistent);
}
