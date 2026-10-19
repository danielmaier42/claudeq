//go:build darwin

#ifndef DISPATCHQ_NOTIFYCLICK_COCOA_H
#define DISPATCHQ_NOTIFYCLICK_COCOA_H

// cqInstallNotifyDelegate makes this process the handler for clicks on DispatchQ
// notifications. Call it before the application finishes launching, so a click
// that launched the app is still delivered.
void cqInstallNotifyDelegate(void);

#endif
