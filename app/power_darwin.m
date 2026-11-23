#import <AppKit/AppKit.h>

extern void marstekPowerEvent(int sleep);

// marstekObservePower subscribes to the NSWorkspace sleep / wake
// notifications. queue:nil runs each block synchronously on the posting
// (main) thread, so the events reach Go in the order macOS sent them.
void marstekObservePower(void) {
	NSNotificationCenter *c = [[NSWorkspace sharedWorkspace] notificationCenter];
	[c addObserverForName:NSWorkspaceWillSleepNotification object:nil queue:nil
	           usingBlock:^(NSNotification *n) { marstekPowerEvent(1); }];
	[c addObserverForName:NSWorkspaceDidWakeNotification object:nil queue:nil
	           usingBlock:^(NSNotification *n) { marstekPowerEvent(0); }];
}
