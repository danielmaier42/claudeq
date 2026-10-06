//go:build darwin

// Objective-C side of "click the notification, get to what it is about": the
// window app registers as the UNUserNotificationCenter delegate for the ClaudeQ
// bundle, so clicking a notification the daemon posted lands here — whether the
// app was already running or the click launched it.
//
// Like menu_cocoa.m this lives in a .m file rather than a cgo preamble, because a
// preamble @implementation is compiled into several object files and the linker
// then rejects the duplicate OBJC class symbols.

#import <AppKit/AppKit.h>
#import <UserNotifications/UserNotifications.h>
#include "notifyclick_cocoa.h"

// Defined in Go (//export). Receives the clicked notification's inbox id and
// its click target — the artifact, run or link it is about, each "" when it
// carries none — and whether the link was already opened in the browser.
extern void goNotificationClicked(char *notificationID, char *artifactID, char *runID, char *link, int openedLink);

// The userInfo keys must match what the daemon sets in
// internal/notify/native_darwin.go.
static NSString *const cqNotificationKey = @"cq_notification";
static NSString *const cqArtifactKey = @"cq_artifact";
static NSString *const cqRunKey = @"cq_run";
static NSString *const cqURLKey = @"cq_url";

// cqUserInfoString returns the string stored under key, or "" when absent or
// not a string.
static NSString *cqUserInfoString(NSDictionary *info, NSString *key) {
    id value = info[key];
    return [value isKindOfClass:[NSString class]] ? (NSString *)value : @"";
}

// cqOpenWebLink opens link in the default browser and reports whether it did.
// Only web links are opened — the daemon already filters, but this is the hand
// that launches things, so it checks too.
static BOOL cqOpenWebLink(NSString *link) {
    if ([link length] == 0) { return NO; }
    NSURL *url = [NSURL URLWithString:link];
    NSString *scheme = [[url scheme] lowercaseString];
    BOOL web = [scheme isEqualToString:@"http"] || [scheme isEqualToString:@"https"];
    return url != nil && web && [[NSWorkspace sharedWorkspace] openURL:url];
}

@interface CQNotifyDelegate : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation CQNotifyDelegate

// The user clicked the notification (or one of its actions).
- (void)userNotificationCenter:(UNUserNotificationCenter *)center
    didReceiveNotificationResponse:(UNNotificationResponse *)response
             withCompletionHandler:(void (^)(void))completionHandler {
    (void)center;
    NSDictionary *info = response.notification.request.content.userInfo;
    NSString *notification = cqUserInfoString(info, cqNotificationKey);
    NSString *artifact = cqUserInfoString(info, cqArtifactKey);
    NSString *run = cqUserInfoString(info, cqRunKey);
    NSString *link = cqUserInfoString(info, cqURLKey);
    // A notification sent with `claudeq notify --url` opens its link (in the
    // default browser) instead of the ClaudeQ window: the link is what the
    // operator was pointed at, and the window has nothing to show for it. If
    // that fails, the link travels on to the page, which tries again from
    // there.
    BOOL opened = cqOpenWebLink(link);
    if (!opened) {
        // The click already activates the app, but the window may be hidden or
        // minimized — bring it to the front so the target is actually visible.
        [NSApp activateIgnoringOtherApps:YES];
        for (NSWindow *win in [NSApp windows]) {
            if ([win isMiniaturized]) { [win deminiaturize:nil]; }
        }
    }
    goNotificationClicked((char *)[notification UTF8String], (char *)[artifact UTF8String],
                          (char *)[run UTF8String], (char *)[link UTF8String], opened ? 1 : 0);
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
