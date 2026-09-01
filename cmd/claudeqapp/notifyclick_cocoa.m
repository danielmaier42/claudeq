//go:build darwin

// Objective-C side of "click the notification, get the artifact": the window app
// registers as the UNUserNotificationCenter delegate for the ClaudeQ bundle, so
// clicking a notification the daemon posted lands here — whether the app was
// already running or the click launched it.
//
// Like menu_cocoa.m this lives in a .m file rather than a cgo preamble, because a
// preamble @implementation is compiled into several object files and the linker
// then rejects the duplicate OBJC class symbols.

#import <AppKit/AppKit.h>
#import <UserNotifications/UserNotifications.h>
#include "notifyclick_cocoa.h"

// Defined in Go (//export). Receives the clicked notification's artifact id, or
// "" when it carried none (a run-outcome notification, which only activates the
// app).
extern void goNotificationClicked(char *artifactID);

// cqArtifactKey must match the userInfo key the daemon sets in
// internal/notify/native_darwin.go.
static NSString *const cqArtifactKey = @"cq_artifact";

@interface CQNotifyDelegate : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation CQNotifyDelegate

// The user clicked the notification (or one of its actions).
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
    didReceiveNotificationResponse:(UNNotificationResponse *)response
             withCompletionHandler:(void (^)(void))completionHandler {
    (void)center;
    NSString *artifact = @"";
    id value = response.notification.request.content.userInfo[cqArtifactKey];
    if ([value isKindOfClass:[NSString class]]) {
        artifact = (NSString *)value;
    }
    // The click already activates the app, but the window may be hidden or
    // minimized — bring it to the front so the artifact is actually visible.
    [NSApp activateIgnoringOtherApps:YES];
    for (NSWindow *win in [NSApp windows]) {
        if ([win isMiniaturized]) { [win deminiaturize:nil]; }
    }
    goNotificationClicked((char *)[artifact UTF8String]);
    completionHandler();
}

// Show the notification even when ClaudeQ itself is the frontmost app; without
// this the system silently swallows it, and a publish while the operator is
// looking at the window would go unannounced.
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
       willPresentNotification:(UNNotification *)notification
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions))completionHandler {
    (void)center;
    (void)notification;
    completionHandler(UNNotificationPresentationOptionBanner |
                      UNNotificationPresentationOptionList |
                      UNNotificationPresentationOptionSound);
}

@end

// Kept alive for the process lifetime: UNUserNotificationCenter holds its
// delegate weakly.
static CQNotifyDelegate *gNotifyDelegate = nil;

void cqInstallNotifyDelegate(void) {
    @try {
        gNotifyDelegate = [[CQNotifyDelegate alloc] init];
        [[UNUserNotificationCenter currentNotificationCenter] setDelegate:gNotifyDelegate];
    } @catch (NSException *ex) { (void)ex; }
}
