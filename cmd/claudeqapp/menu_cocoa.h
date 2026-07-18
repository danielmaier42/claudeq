//go:build darwin

#ifndef CLAUDEQ_MENU_COCOA_H
#define CLAUDEQ_MENU_COCOA_H

// cqInstallMenu builds claudeq's native menu bar and installs it as the
// application's main menu (safe to call from any thread; hops to main).
void cqInstallMenu(void);

#endif
