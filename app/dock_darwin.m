#import <AppKit/AppKit.h>

// The app icon art is a full-bleed tile (960 of 1024 px, the Windows grid).
// A Dock image set at run time is shown as is, while macOS fits bundle icons
// to its own grid (the tile about 80% of the canvas, 824 of 1024). The tile
// is drawn at that size on a 1024 px bitmap, so the on and off images match
// each other and the icons around them.
static const NSInteger kDockPx = 1024;
static const CGFloat kArtTile = 960.0 / 1024.0;   // tile share of the source art
static const CGFloat kAppleTile = 824.0 / 1024.0; // tile share on the macOS grid

static NSImage *dockImage(const void *png, int n) {
	NSImage *src = [[NSImage alloc] initWithData:[NSData dataWithBytes:png length:(NSUInteger)n]];
	if (src == nil) {
		return nil;
	}
	NSBitmapImageRep *rep = [[NSBitmapImageRep alloc]
		initWithBitmapDataPlanes:NULL pixelsWide:kDockPx pixelsHigh:kDockPx
		bitsPerSample:8 samplesPerPixel:4 hasAlpha:YES isPlanar:NO
		colorSpaceName:NSDeviceRGBColorSpace bytesPerRow:0 bitsPerPixel:0];
	CGFloat side = kDockPx * kAppleTile / kArtTile;
	CGFloat inset = (kDockPx - side) / 2.0;
	[NSGraphicsContext saveGraphicsState];
	NSGraphicsContext *ctx = [NSGraphicsContext graphicsContextWithBitmapImageRep:rep];
	[NSGraphicsContext setCurrentContext:ctx];
	[ctx setImageInterpolation:NSImageInterpolationHigh];
	[src drawInRect:NSMakeRect(inset, inset, side, side)
	       fromRect:NSZeroRect
	      operation:NSCompositingOperationCopy
	       fraction:1.0];
	[NSGraphicsContext restoreGraphicsState];
	[src release];
	NSImage *dst = [[NSImage alloc] initWithSize:NSMakeSize(kDockPx, kDockPx)];
	[dst addRepresentation:rep];
	[rep release];
	return [dst autorelease];
}

// marstekSetDock shows the Dock icon (regular activation policy) with the
// given PNG, or hides it (accessory). Main thread only.
void marstekSetDock(int shown, const void *png, int n) {
	if (!shown) {
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
		return;
	}
	BOOL appearing = [NSApp activationPolicy] != NSApplicationActivationPolicyRegular;
	if (appearing) {
		// The Dock tile appears with the bundle icon; the image is set after
		// it, or the Dock keeps the bundle icon.
		[NSApp setActivationPolicy:NSApplicationActivationPolicyRegular];
	}
	if (png != NULL && n > 0) {
		@autoreleasepool {
			NSImage *img = dockImage(png, n);
			if (img != nil) {
				[NSApp setApplicationIconImage:img];
			}
		}
	}
	if (appearing) {
		// A regular app that just appeared in the Dock comes to the front
		// with its window (opened from the menu bar, a toast, a second launch).
		[NSApp activateIgnoringOtherApps:YES];
	}
}
