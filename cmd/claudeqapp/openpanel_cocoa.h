//go:build darwin

#ifndef CLAUDEQ_OPENPANEL_COCOA_H
#define CLAUDEQ_OPENPANEL_COCOA_H

// cqInstallOpenPanel makes <input type="file"> work in the WKWebView inside
// the given NSWindow (as returned by webview's Window()) by installing a
// retained WKUIDelegate that answers file-chooser requests with an NSOpenPanel
// sheet. Returns 1 when a WKWebView was found and wired, 0 otherwise.
int cqInstallOpenPanel(void *nsWindow);

#endif
