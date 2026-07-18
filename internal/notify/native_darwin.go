//go:build darwin

package notify

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework UserNotifications
#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>
#include <stdlib.h>
#include <dispatch/dispatch.h>

// cqNotifyAvailable reports whether the process runs inside an app bundle with a
// bundle identifier — a prerequisite for UNUserNotificationCenter, which throws
// for a bare (non-bundled) executable. A bare dev binary returns 0, so callers
// fall back to osascript.
static int cqNotifyAvailable(void) {
    NSString *bid = [[NSBundle mainBundle] bundleIdentifier];
    return (bid != nil && [bid length] > 0) ? 1 : 0;
}

// cqRequestNotifyAuth asks for notification permission (the one-time prompt).
// Best-effort and asynchronous; safe to call at startup.
static void cqRequestNotifyAuth(void) {
    @try {
        UNUserNotificationCenter *c = [UNUserNotificationCenter currentNotificationCenter];
        [c requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
                         completionHandler:^(BOOL granted, NSError *e){ (void)granted; (void)e; }];
    } @catch (NSException *ex) { (void)ex; }
}

// cqPostNotification posts a notification through the app bundle so it carries
// the app's icon. Returns 0 on success, non-zero on failure.
static int cqPostNotification(const char *ctitle, const char *cbody) {
    @try {
        UNUserNotificationCenter *c = [UNUserNotificationCenter currentNotificationCenter];
        UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
        content.title = [NSString stringWithUTF8String:(ctitle ? ctitle : "")];
        content.body  = [NSString stringWithUTF8String:(cbody ? cbody : "")];
        content.sound = [UNNotificationSound defaultSound];
        UNNotificationRequest *req =
            [UNNotificationRequest requestWithIdentifier:[[NSUUID UUID] UUIDString]
                                                 content:content
                                                 trigger:nil];
        __block int rc = 0;
        dispatch_semaphore_t sem = dispatch_semaphore_create(0);
        [c addNotificationRequest:req withCompletionHandler:^(NSError *e){
            if (e) { rc = 2; }
            dispatch_semaphore_signal(sem);
        }];
        dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, 2LL * NSEC_PER_SEC));
        return rc;
    } @catch (NSException *ex) { (void)ex; return 1; }
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func nativeNotifyAvailable() bool { return C.cqNotifyAvailable() == 1 }

func requestNativeAuth() { C.cqRequestNotifyAuth() }

func postNativeNotification(title, body string) error {
	ct := C.CString(title)
	defer C.free(unsafe.Pointer(ct))
	cb := C.CString(body)
	defer C.free(unsafe.Pointer(cb))
	if rc := C.cqPostNotification(ct, cb); rc != 0 {
		return fmt.Errorf("native notification failed (rc=%d)", int(rc))
	}
	return nil
}
