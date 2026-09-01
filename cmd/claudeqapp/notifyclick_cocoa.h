//go:build darwin

#ifndef CLAUDEQ_NOTIFYCLICK_COCOA_H
#define CLAUDEQ_NOTIFYCLICK_COCOA_H

// cqInstallNotifyDelegate makes this process the handler for clicks on ClaudeQ
// notifications. Call it before the application finishes launching, so a click
// that launched the app is still delivered.
void cqInstallNotifyDelegate(void);

#endif
