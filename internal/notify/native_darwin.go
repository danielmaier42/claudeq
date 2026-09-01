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

// cqAuthorizationStatus reports whether macOS will actually show this app's
// notifications. A "denied" app can post all it likes: the record is stored and
// presented as none, with nothing on screen.
static const char *cqAuthorizationStatus(void) {
    @try {
        __block const char *status = "unknown";
        dispatch_semaphore_t sem = dispatch_semaphore_create(0);
        [[UNUserNotificationCenter currentNotificationCenter]
            getNotificationSettingsWithCompletionHandler:^(UNNotificationSettings *s){
                switch (s.authorizationStatus) {
                    case UNAuthorizationStatusNotDetermined: status = "not_determined"; break;
                    case UNAuthorizationStatusDenied:        status = "denied";         break;
                    case UNAuthorizationStatusAuthorized:    status = "authorized";     break;
                    case UNAuthorizationStatusProvisional:   status = "provisional";    break;
                    default:                                 status = "unknown";        break;
                }
                dispatch_semaphore_signal(sem);
            }];
        if (dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, 2LL * NSEC_PER_SEC)) != 0) {
            return "unknown";
        }
        return status;
    } @catch (NSException *ex) { (void)ex; return "unknown"; }
}

// cqPostNotification posts a notification through the app bundle so it carries
// the app's icon. A non-empty cartifact travels along in the userInfo, where the
// window app picks it up when the user clicks the notification (the key is kept
// in sync with cqArtifactKey in cmd/claudeqapp/notifyclick_cocoa.m).
// Returns 0 on success, non-zero on failure.
static int cqPostNotification(const char *ctitle, const char *cbody, const char *cartifact) {
    @try {
        UNUserNotificationCenter *c = [UNUserNotificationCenter currentNotificationCenter];
        UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
        content.title = [NSString stringWithUTF8String:(ctitle ? ctitle : "")];
        content.body  = [NSString stringWithUTF8String:(cbody ? cbody : "")];
        content.sound = [UNNotificationSound defaultSound];
        if (cartifact && cartifact[0] != '\0') {
            content.userInfo = @{ @"cq_artifact": [NSString stringWithUTF8String:cartifact] };
        }
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

func nativeAuthorizationStatus() string { return C.GoString(C.cqAuthorizationStatus()) }

func postNativeNotification(title, body, artifactID string) error {
	ct := C.CString(title)
	defer C.free(unsafe.Pointer(ct))
	cb := C.CString(body)
	defer C.free(unsafe.Pointer(cb))
	ca := C.CString(artifactID)
	defer C.free(unsafe.Pointer(ca))
	if rc := C.cqPostNotification(ct, cb, ca); rc != 0 {
		return fmt.Errorf("native notification failed (rc=%d)", int(rc))
	}
	return nil
}
