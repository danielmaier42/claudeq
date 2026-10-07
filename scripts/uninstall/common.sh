# common.sh — shared by the uninstaller's postinstall scripts (sourced, not
# run). It resolves the user ClaudeQ belongs to: the installer runs the scripts
# as root, but the LaunchAgent, the processes and the data are the logged-in
# user's.
#
# CLAUDEQ_CONSOLE_USER and CLAUDEQ_CONSOLE_HOME stand in for the console user
# and their home in scripts/acceptance.sh. The installer runs scripts with a
# clean environment, so a real uninstall never sees them.

LABEL="de.maierdaniel.claudeq"

consoleUser="${CLAUDEQ_CONSOLE_USER:-$(stat -f "%Su" /dev/console 2>/dev/null || echo "")}"
uid=""
homeDir=""
if [ -n "$consoleUser" ] && [ "$consoleUser" != "root" ]; then
	uid=$(id -u "$consoleUser" 2>/dev/null || echo "")
	homeDir="${CLAUDEQ_CONSOLE_HOME:-$(dscl . -read "/Users/$consoleUser" NFSHomeDirectory 2>/dev/null | awk '{print $2}')}"
fi
# An empty or unusable home would turn "$homeDir/Library/..." into a path
# under the system's /Library, which these scripts must never touch.
if [ -z "$homeDir" ] || [ "$homeDir" = "/" ] || [ ! -d "$homeDir" ]; then
	homeDir=""
fi

# as_user runs a command in the console user's GUI session, as them.
as_user() {
	launchctl asuser "$uid" sudo -u "$consoleUser" "$@"
}
