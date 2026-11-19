#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#import <PDFKit/PDFKit.h>
#include "webkit_darwin.h"

extern void inkWKDone(uintptr_t handle, void *data, int length, char *err);

static void done(uintptr_t h, NSData *data, NSString *err) {
	inkWKDone(h, (void *)data.bytes, (int)data.length, err ? (char *)err.UTF8String : NULL);
}

// onMain runs block on the main thread, waiting for it.
static void onMain(dispatch_block_t block) {
	if ([NSThread isMainThread]) block();
	else dispatch_sync(dispatch_get_main_queue(), block);
}

@interface InkWK : NSObject <WKNavigationDelegate>
@property(strong) NSWindow *window;
@property(strong) WKWebView *web;
@property uintptr_t loading;
// zoom is view points per CSS pixel.
@property double zoom;
@end

@implementation InkWK
- (void)finish:(NSString *)err {
	uintptr_t h = self.loading;
	self.loading = 0;
	if (h) done(h, nil, err);
}
- (void)webView:(WKWebView *)w didFinishNavigation:(WKNavigation *)n { [self finish:nil]; }
- (void)webView:(WKWebView *)w didFailNavigation:(WKNavigation *)n withError:(NSError *)e {
	[self finish:e.localizedDescription];
}
- (void)webView:(WKWebView *)w didFailProvisionalNavigation:(WKNavigation *)n withError:(NSError *)e {
	[self finish:e.localizedDescription];
}
- (void)webViewWebContentProcessDidTerminate:(WKWebView *)w { [self finish:@"the web content process stopped"]; }
@end

void *ink_wk_new(double width, double height, double zoom) {
	__block InkWK *k;
	onMain(^{
		k = [InkWK new];
		k.zoom = zoom;
		WKWebViewConfiguration *cfg = [WKWebViewConfiguration new];
		// The deck refers to the pack's fonts and images by file: URL.
		[cfg.preferences setValue:@YES forKey:@"allowFileAccessFromFileURLs"];
		// Far off screen, and never ordered in: it draws, but nobody sees it.
		k.window = [[NSWindow alloc] initWithContentRect:NSMakeRect(-30000, -30000, width * zoom, height * zoom)
		                                       styleMask:NSWindowStyleMaskBorderless
		                                         backing:NSBackingStoreBuffered
		                                           defer:NO];
		k.window.releasedWhenClosed = NO;
		k.web = [[WKWebView alloc] initWithFrame:NSMakeRect(0, 0, width * zoom, height * zoom) configuration:cfg];
		// The page still lays out width CSS pixels across.
		k.web.pageZoom = zoom;
		// No white behind the page, so pictures of parts keep transparency.
		[k.web setValue:@NO forKey:@"drawsBackground"];
		k.web.navigationDelegate = k;
		k.window.contentView = k.web;
	});
	return (__bridge_retained void *)k;
}

void ink_wk_load(void *view, const char *file, const char *readAccess, uintptr_t handle) {
	InkWK *k = (__bridge InkWK *)view;
	NSString *f = [NSString stringWithUTF8String:file], *r = [NSString stringWithUTF8String:readAccess];
	dispatch_async(dispatch_get_main_queue(), ^{
		k.loading = handle;
		[k.web loadFileURL:[NSURL fileURLWithPath:f] allowingReadAccessToURL:[NSURL fileURLWithPath:r isDirectory:YES]];
	});
}

void ink_wk_eval(void *view, const char *body, uintptr_t handle) {
	InkWK *k = (__bridge InkWK *)view;
	NSString *js = [NSString stringWithUTF8String:body];
	dispatch_async(dispatch_get_main_queue(), ^{
		[k.web callAsyncJavaScript:js
		                 arguments:@{}
		                   inFrame:nil
		            inContentWorld:WKContentWorld.pageWorld
		         completionHandler:^(id result, NSError *error) {
			NSString *s = [result isKindOfClass:NSString.class] ? result : (result ? [result description] : @"");
			done(handle, [s dataUsingEncoding:NSUTF8StringEncoding], error ? error.localizedDescription : nil);
		}];
	});
}

void ink_wk_snapshot(void *view, double x, double y, double w, double h, double scale, int transparent, uintptr_t handle) {
	InkWK *k = (__bridge InkWK *)view;
	dispatch_async(dispatch_get_main_queue(), ^{
		WKSnapshotConfiguration *c = [WKSnapshotConfiguration new];
		double z = k.zoom;
		c.rect = NSMakeRect(x * z, y * z, w * z, h * z);
		c.afterScreenUpdates = YES;
		CGFloat backing = k.window.backingScaleFactor ?: 1;
		c.snapshotWidth = @(w * scale / backing);
		[k.web takeSnapshotWithConfiguration:c completionHandler:^(NSImage *img, NSError *error) {
			if (!img) {
				done(handle, nil, error ? error.localizedDescription : @"no snapshot");
				return;
			}
			// Exactly scale pixels per CSS pixel, whatever the screen.
			NSInteger pw = MAX(1, llround(w * scale)), ph = MAX(1, llround(h * scale));
			NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithBitmapDataPlanes:NULL
			    pixelsWide:pw pixelsHigh:ph bitsPerSample:8 samplesPerPixel:4 hasAlpha:YES isPlanar:NO
			    colorSpaceName:NSDeviceRGBColorSpace bytesPerRow:0 bitsPerPixel:0];
			rep.size = NSMakeSize(w, h);
			[NSGraphicsContext saveGraphicsState];
			NSGraphicsContext.currentContext = [NSGraphicsContext graphicsContextWithBitmapImageRep:rep];
			if (!transparent) {
				[NSColor.whiteColor setFill];
				NSRectFill(NSMakeRect(0, 0, w, h));
			}
			[img drawInRect:NSMakeRect(0, 0, w, h)];
			[NSGraphicsContext restoreGraphicsState];
			done(handle, [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}], nil);
		}];
	});
}

void ink_wk_pdf(void *view, double width, double height, uintptr_t handle) {
	InkWK *k = (__bridge InkWK *)view;
	dispatch_async(dispatch_get_main_queue(), ^{
		WKPDFConfiguration *c = [WKPDFConfiguration new];
		c.rect = NSMakeRect(0, 0, width * k.zoom, height * k.zoom);
		[k.web createPDFWithConfiguration:c completionHandler:^(NSData *pdf, NSError *error) {
			done(handle, pdf, error ? error.localizedDescription : nil);
		}];
	});
}

void ink_wk_free(void *view) {
	InkWK *k = (__bridge_transfer InkWK *)view;
	dispatch_async(dispatch_get_main_queue(), ^{
		k.web.navigationDelegate = nil;
		[k.window close];
		k.window.contentView = nil;
	});
}

void ink_pdf_merge(void **pages, int *lengths, int n, uintptr_t handle) {
	@autoreleasepool {
		PDFDocument *out = [PDFDocument new];
		// A page keeps a weak link to its document, so the sources live
		// until the merged document is written.
		NSMutableArray *keep = [NSMutableArray array];
		for (int i = 0; i < n; i++) {
			PDFDocument *d = [[PDFDocument alloc] initWithData:[NSData dataWithBytes:pages[i] length:lengths[i]]];
			if (!d) {
				done(handle, nil, [NSString stringWithFormat:@"page %d is not a PDF", i + 1]);
				return;
			}
			[keep addObject:d];
			for (NSUInteger p = 0; p < d.pageCount; p++) [out insertPage:[d pageAtIndex:p] atIndex:out.pageCount];
		}
		done(handle, out.dataRepresentation, nil);
	}
}

void ink_run_main(void) {
	[NSApplication sharedApplication];
	[NSApp setActivationPolicy:NSApplicationActivationPolicyProhibited];
	[NSApp run];
}
