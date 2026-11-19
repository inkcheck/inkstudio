// The C side of package webkit: an offscreen WKWebView driven from Go.
// Every call that touches the view runs on the main thread. Results come
// back through inkWKDone, with the handle the call was given.
#include <stdint.h>

void *ink_wk_new(double width, double height);
void ink_wk_load(void *view, const char *file, const char *readAccess, uintptr_t handle);
void ink_wk_eval(void *view, const char *body, uintptr_t handle);
void ink_wk_snapshot(void *view, double x, double y, double w, double h, double scale, int transparent, uintptr_t handle);
void ink_wk_pdf(void *view, double width, double height, uintptr_t handle);
void ink_wk_free(void *view);
void ink_pdf_merge(void **pages, int *lengths, int n, uintptr_t handle);
void ink_run_main(void);
