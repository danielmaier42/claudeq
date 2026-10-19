//go:build darwin

#ifndef DISPATCHQ_MENU_COCOA_H
#define DISPATCHQ_MENU_COCOA_H

// cqInstallMenu builds dispatchq's native menu bar and installs it as the
// application's main menu (safe to call from any thread; hops to main).
void cqInstallMenu(void);

#endif
