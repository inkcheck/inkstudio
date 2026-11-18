// The live preview: the rendered deck in a frame that shows one slide.
// A new render loads in a hidden frame and replaces the visible one once it
// has loaded, so the preview never flashes blank while you type.
//
// The frame gets the deck through srcdoc, not src. WebKit, which the app
// runs in on macOS, won't navigate a frame to the app's own wails:// scheme,
// so the page is fetched here and handed over. An srcdoc frame shares this
// page's origin, so its images, fonts and styles still load from the app.

declare global {
  interface Window {
    /** Set up by the preview script in the deck page (studio/preview.go). */
    studioShow?: (n: number) => void;
  }
}

export class Preview {
  private host: HTMLElement;
  private front: HTMLIFrameElement | null = null;
  private loading: HTMLIFrameElement | null = null;
  private url = "";
  private slide = 1;
  private mode: "slide" | "strip";

  /** A strip shows every slide as thumbnails; a slide preview shows one. */
  constructor(host: HTMLElement, mode: "slide" | "strip" = "slide") {
    this.host = host;
    this.mode = mode;
  }

  /** Loads a rendered deck, keeping the current slide. */
  load(url: string) {
    if (url === this.url) return;
    this.url = url;
    void this.fetchAndShow(url);
  }

  private async fetchAndShow(url: string) {
    let html: string;
    try {
      const res = await fetch(url);
      if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
      html = await res.text();
    } catch {
      return; // a newer render will follow, or the error shows elsewhere
    }
    if (url !== this.url) return; // superseded while fetching

    // The page reads its mode and first slide from these.
    const config = `<script>window.studioMode=${JSON.stringify(this.mode)};window.studioSlide=${this.slide};</script>`;
    html = html.replace(/<head>/i, `<head>${config}`);

    this.loading?.remove();
    const frame = document.createElement("iframe");
    frame.title = this.mode === "strip" ? "Slide thumbnails" : "Slide preview";
    frame.className = "loading";
    frame.setAttribute("tabindex", "-1");
    this.loading = frame;
    frame.addEventListener(
      "load",
      () => {
        if (this.loading !== frame) return;
        this.loading = null;
        // The slide may have changed while it loaded.
        this.call(frame, this.slide);
        // Keep the scroll position, for the thumbnail strip.
        try {
          const y = this.front?.contentWindow?.scrollY ?? 0;
          if (y) frame.contentWindow?.scrollTo(0, y);
        } catch {
          // No old frame to copy from.
        }
        frame.className = "";
        this.front?.remove();
        this.front = frame;
      },
      { once: true },
    );
    frame.srcdoc = html;
    this.host.appendChild(frame);
  }

  /** Shows slide n, from 1. */
  show(n: number) {
    this.slide = n;
    for (const f of [this.front, this.loading]) if (f) this.call(f, n);
  }

  /** Lays the slide out again, after the frame was hidden or resized. */
  refit() {
    try {
      this.front?.contentWindow?.dispatchEvent(new Event("resize"));
    } catch {
      // Not loaded yet.
    }
  }

  get loaded() {
    return this.url;
  }

  clear() {
    this.front?.remove();
    this.loading?.remove();
    this.front = this.loading = null;
    this.url = "";
  }

  private call(frame: HTMLIFrameElement, n: number) {
    try {
      frame.contentWindow?.studioShow?.(n);
    } catch {
      // Not loaded yet; the page starts on window.studioSlide.
    }
  }
}
