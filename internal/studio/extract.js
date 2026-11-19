// Reads one rendered slide as editable parts, for slidedoc.Slide: the
// browser has already laid it out, so positions, fonts and colours come from
// it. Text becomes text boxes, solid boxes and borders become rectangles,
// and images, SVG and gradient backgrounds are marked for capture as
// pictures (data-ink-cap), which Go takes afterwards.
//
// Called as inkExtract(index). Plain ES2017, so it runs in any engine.
(function () {
  var nextCap = 1;

  function parseColor(s, opacity) {
    var m = /rgba?\(\s*([\d.]+)[ ,]+([\d.]+)[ ,]+([\d.]+)(?:\s*[,/]\s*([\d.]+%?))?\s*\)/.exec(s || '');
    if (!m) return null;
    var a = m[4] === undefined ? 1 : m[4].slice(-1) === '%' ? parseFloat(m[4]) / 100 : parseFloat(m[4]);
    a *= opacity === undefined ? 1 : opacity;
    if (a <= 0.003) return null;
    return { r: Math.round(+m[1]), g: Math.round(+m[2]), b: Math.round(+m[3]), a: Math.round(a * 1000) / 1000 };
  }

  function opacityOf(el, stop) {
    var o = 1;
    for (var e = el; e && e !== stop; e = e.parentElement) o *= parseFloat(getComputedStyle(e).opacity) || 0;
    return o;
  }

  var GENERIC = { 'sans-serif': 'Arial', 'system-ui': 'Helvetica Neue', '-apple-system': 'Helvetica Neue',
    'serif': 'Georgia', 'monospace': 'Menlo', 'ui-monospace': 'Menlo', 'ui-sans-serif': 'Helvetica Neue',
    'ui-serif': 'Georgia', 'cursive': 'Comic Sans MS', 'fantasy': 'Impact' };
  // The family the browser really used: the first in the list that is
  // installed or loaded. A missing font in the file would make PowerPoint
  // pick its own, often a serif.
  var fontCache = {};
  var canvas = document.createElement('canvas').getContext('2d');
  function available(name) {
    if (name in fontCache) return fontCache[name];
    var sample = 'mmmmmmmmmmlli1WQ@#';
    var ok = ['monospace', 'serif', 'sans-serif'].some(function (base) {
      canvas.font = '72px ' + base;
      var w0 = canvas.measureText(sample).width;
      canvas.font = '72px "' + name + '", ' + base;
      return canvas.measureText(sample).width !== w0;
    });
    return (fontCache[name] = ok);
  }
  // System fonts that Office apps do not know, and what they look like.
  var SYSTEM = { 'SFMono-Regular': 'Menlo', 'SF Mono': 'Menlo', 'SF Pro': 'Helvetica Neue',
    'SF Pro Text': 'Helvetica Neue', 'SF Pro Display': 'Helvetica Neue', 'BlinkMacSystemFont': 'Helvetica Neue' };
  function fontName(family) {
    var names = (family || '').split(',').map(function (f) { return f.trim().replace(/^["']|["']$/g, ''); });
    for (var i = 0; i < names.length; i++) {
      var n = names[i];
      if (GENERIC[n]) return GENERIC[n];
      if (n && available(n)) return SYSTEM[n] || n;
    }
    return 'Arial';
  }
  function isMono(family) { return /mono|code|consol|courier|menlo/i.test(family || ''); }

  function px(v) { var n = parseFloat(v); return isFinite(n) ? n : 0; }

  // Pseudo-elements with content cannot be measured, so each becomes a real
  // span with the pseudo-element's styles, and the pseudo-element goes.
  function materialize(root) {
    if (root.hasAttribute('data-ink-mat')) return;
    root.setAttribute('data-ink-mat', '');
    var els = Array.prototype.slice.call(root.querySelectorAll('*'));
    var made = [];
    els.forEach(function (el) {
      ['::before', '::after'].forEach(function (pseudo) {
        var cs = getComputedStyle(el, pseudo);
        var content = cs.content;
        if (!content || content === 'none' || content === 'normal') return;
        var text = pseudoText(el, content);
        if (text === null) return;
        var span = document.createElement('span');
        span.textContent = text;
        for (var i = 0; i < cs.length; i++) {
          var prop = cs[i];
          if (prop === 'content') continue;
          span.style.setProperty(prop, cs.getPropertyValue(prop));
        }
        span.setAttribute('data-ink-pseudo', '');
        made.push({ el: el, span: span, before: pseudo === '::before' });
      });
    });
    made.forEach(function (m) {
      if (m.before) m.el.insertBefore(m.span, m.el.firstChild);
      else m.el.appendChild(m.span);
      m.el.setAttribute('data-ink-nopseudo', '');
    });
    var style = document.createElement('style');
    style.textContent = '[data-ink-nopseudo]::before, [data-ink-nopseudo]::after { content: none !important; }';
    document.head.appendChild(style);
  }

  // The text of a pseudo-element's content: strings, and the counters
  // templates use for numbered items.
  function pseudoText(el, content) {
    var out = '';
    var re = /"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)'|counter\(\s*[\w-]+\s*(?:,\s*([\w-]+))?\s*\)|attr\(\s*([\w-]+)\s*\)/g;
    var m, any = false;
    while ((m = re.exec(content))) {
      any = true;
      if (m[1] !== undefined || m[2] !== undefined) {
        out += (m[1] !== undefined ? m[1] : m[2]).replace(/\\([0-9a-fA-F]{1,6})\s?/g, function (_, h) {
          return String.fromCodePoint(parseInt(h, 16));
        }).replace(/\\(.)/g, '$1');
      } else if (m[4]) {
        out += el.getAttribute(m[4]) || '';
      } else {
        // A counter: the element's place among its siblings of its kind.
        var n = 1;
        for (var s = el.previousElementSibling; s; s = s.previousElementSibling) if (s.tagName === el.tagName) n++;
        var style = m[3] || 'decimal';
        if (style === 'decimal-leading-zero') out += (n < 10 ? '0' : '') + n;
        else if (style === 'lower-alpha' || style === 'lower-latin') out += String.fromCharCode(96 + n);
        else if (style === 'upper-alpha' || style === 'upper-latin') out += String.fromCharCode(64 + n);
        else if (style === 'lower-roman') out += roman(n).toLowerCase();
        else if (style === 'upper-roman') out += roman(n);
        else out += n;
      }
    }
    return any ? out : null;
  }

  function roman(n) {
    var r = '', v = [1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1],
      s = ['M', 'CM', 'D', 'CD', 'C', 'XC', 'L', 'XL', 'X', 'IX', 'V', 'IV', 'I'];
    for (var i = 0; i < v.length; i++) while (n >= v[i]) { r += s[i]; n -= v[i]; }
    return r;
  }

  function isInline(el) {
    var d = getComputedStyle(el).display;
    return d === 'inline' || d === 'contents';
  }
  function isReplaced(el) {
    return /^(IMG|SVG|CANVAS|VIDEO|PICTURE|IFRAME|OBJECT|svg)$/.test(el.tagName);
  }
  function hidden(el) {
    var cs = getComputedStyle(el);
    return cs.display === 'none' || cs.visibility === 'hidden';
  }

  // The text nodes and breaks of a block's own inline content, in order,
  // stopping at block children and pictures.
  function inlineNodes(block) {
    var out = [];
    (function walk(node) {
      for (var c = node.firstChild; c; c = c.nextSibling) {
        if (c.nodeType === 3) {
          if (c.data.length) out.push(c);
        } else if (c.nodeType === 1) {
          if (hidden(c) || isReplaced(c)) continue;
          if (c.tagName === 'BR') out.push(c);
          else if (isInline(c)) walk(c);
        }
      }
    })(block);
    return out;
  }

  function hasOwnText(block) {
    var nodes = inlineNodes(block);
    for (var i = 0; i < nodes.length; i++) if (nodes[i].nodeType === 3 && /\S/.test(nodes[i].data)) return true;
    return false;
  }

  function lineHeight(cs) {
    var lh = cs.lineHeight;
    if (lh === 'normal') return px(cs.fontSize) * 1.2;
    return px(lh);
  }

  function applyTransform(text, t) {
    if (t === 'uppercase') return text.toUpperCase();
    if (t === 'lowercase') return text.toLowerCase();
    if (t === 'capitalize') return text.replace(/(^|\s)(\S)/g, function (_, a, b) { return a + b.toUpperCase(); });
    return text;
  }

  // A paragraph from a block: its runs, and the boxes of its lines.
  function paragraph(block, page, origin) {
    var bcs = getComputedStyle(block);
    var pre = /^pre/.test(bcs.whiteSpace);
    var runs = [];
    var rects = [];
    inlineNodes(block).forEach(function (n) {
      if (n.nodeType === 1) {
        var bs = getComputedStyle(n.parentElement);
        runs.push({ text: '', break: true, font: fontName(bs.fontFamily), size: px(bs.fontSize) });
        return;
      }
      var el = n.parentElement;
      var cs = getComputedStyle(el);
      var text = n.data;
      if (!pre) text = text.replace(/[\t\n\r ]+/g, ' ');
      text = applyTransform(text, cs.textTransform);
      var range = document.createRange();
      range.selectNodeContents(n);
      var rs = range.getClientRects();
      for (var i = 0; i < rs.length; i++) if (rs[i].width > 0 && rs[i].height > 0) rects.push(rs[i]);
      var link = el.closest('a[href]');
      var deco = cs.textDecorationLine || cs.textDecoration || '';
      var ls = cs.letterSpacing === 'normal' ? 0 : px(cs.letterSpacing);
      runs.push({
        text: text,
        font: fontName(cs.fontFamily),
        mono: isMono(cs.fontFamily),
        size: px(cs.fontSize),
        bold: (parseInt(cs.fontWeight, 10) || 400) >= 600,
        italic: cs.fontStyle === 'italic' || cs.fontStyle === 'oblique',
        underline: deco.indexOf('underline') >= 0,
        strike: deco.indexOf('line-through') >= 0,
        color: parseColor(cs.color, opacityOf(el, page)),
        spacing: ls || undefined,
        href: link ? link.href : undefined,
      });
    });
    if (!pre) {
      // Collapse spaces across runs, and trim the paragraph's ends.
      var prevSpace = true;
      runs.forEach(function (r) {
        if (r.break) { prevSpace = true; return; }
        if (prevSpace) r.text = r.text.replace(/^ /, '');
        if (r.text.length) prevSpace = / $/.test(r.text);
      });
      for (var i = runs.length - 1; i >= 0; i--) {
        if (runs[i].break) break;
        runs[i].text = runs[i].text.replace(/ +$/, '');
        if (runs[i].text.length) break;
      }
    } else {
      runs.forEach(function (r) { r.text = r.text.replace(/\n$/, ''); });
      // A line break in preformatted text is a break run.
      var split = [];
      runs.forEach(function (r) {
        if (r.break || r.text.indexOf('\n') < 0) { split.push(r); return; }
        r.text.split('\n').forEach(function (part, i) {
          if (i > 0) split.push({ text: '', break: true, font: r.font, size: r.size });
          if (part.length) split.push(Object.assign({}, r, { text: part }));
        });
      });
      runs = split;
    }
    runs = runs.filter(function (r) { return r.break || r.text.length; });
    if (!rects.length) return null;
    var top = Infinity, bottom = -Infinity, left = Infinity, right = -Infinity;
    rects.forEach(function (r) {
      top = Math.min(top, r.top); bottom = Math.max(bottom, r.bottom);
      left = Math.min(left, r.left); right = Math.max(right, r.right);
    });
    var lh = lineHeight(bcs);
    var first = rects[0];
    // The line box is taller than the glyphs by the half-leading.
    var lead = Math.max(0, (lh - first.height) / 2);
    var align = { center: 'ctr', right: 'r', end: 'r', justify: 'just' }[bcs.textAlign] || 'l';
    return {
      para: { align: align, lineHeight: lh, runs: runs },
      top: top - lead - origin.top,
      bottom: bottom + lead - origin.top,
      left: left - origin.left,
      right: right - origin.left,
      pre: pre,
    };
  }

  function contentBox(el, origin) {
    var r = el.getBoundingClientRect();
    var cs = getComputedStyle(el);
    var l = r.left + px(cs.borderLeftWidth) + px(cs.paddingLeft);
    var t = r.top + px(cs.borderTopWidth) + px(cs.paddingTop);
    var w = r.width - px(cs.borderLeftWidth) - px(cs.paddingLeft) - px(cs.borderRightWidth) - px(cs.paddingRight);
    return { x: l - origin.left, y: t - origin.top, w: w };
  }

  function bulletFor(li) {
    var cs = getComputedStyle(li);
    var t = cs.listStyleType;
    if (t === 'none') return '';
    if (t === 'disc') return '•';
    if (t === 'circle') return '◦';
    if (t === 'square') return '▪';
    if (/^"/.test(t)) return t.slice(1, -1);
    return '1.';
  }

  window.inkExtract = function (index) {
    var page = document.querySelectorAll('.page')[index];
    materialize(page);
    var slideEl = page.querySelector('.slide') || page;
    var origin = page.getBoundingClientRect();
    var items = [];
    var slide = { background: null, backgroundCap: 0, items: items };

    function cap(el, mode) {
      var id = nextCap++;
      el.setAttribute('data-ink-cap', id + ':' + mode);
      return id;
    }
    function box(el) {
      var r = el.getBoundingClientRect();
      return [r.left - origin.left, r.top - origin.top, r.width, r.height];
    }

    var scs = getComputedStyle(slideEl);
    slide.background = parseColor(scs.backgroundColor);
    if (scs.backgroundImage !== 'none') slide.backgroundCap = cap(slideEl, 'bg');

    function decorate(el, cs) {
      var b = box(el);
      if (b[2] <= 0 || b[3] <= 0) return;
      var op = opacityOf(el, page);
      var mask = cs.maskImage || cs.webkitMaskImage;
      if (mask && mask !== 'none') {
        // A shape cut out by a mask, such as a logo: only a picture keeps it.
        items.push({ kind: 'image', box: b, cap: cap(el, 'el') });
        return true;
      }
      if (cs.backgroundImage && cs.backgroundImage !== 'none') {
        items.push({ kind: 'image', box: b, cap: cap(el, 'bg') });
      }
      var fill = parseColor(cs.backgroundColor, op);
      var sides = ['Top', 'Right', 'Bottom', 'Left'].map(function (s) {
        var w = cs['border' + s + 'Style'] === 'none' || cs['border' + s + 'Style'] === 'hidden' ? 0 : px(cs['border' + s + 'Width']);
        return { w: w, c: w ? parseColor(cs['border' + s + 'Color'], op) : null };
      });
      var radius = px(cs.borderTopLeftRadius);
      var uniform = sides.every(function (s) {
        return s.w === sides[0].w && JSON.stringify(s.c) === JSON.stringify(sides[0].c);
      }) && sides[0].w > 0 && sides[0].c;
      if (fill || uniform) {
        var it = { kind: 'rect', box: b, radius: radius || undefined };
        if (fill) it.fill = fill;
        if (uniform) {
          it.line = sides[0].c;
          it.lineWidth = sides[0].w;
          // An outline is drawn on the edge; CSS borders sit inside it.
          var h = sides[0].w / 2;
          it.box = [b[0] + h, b[1] + h, b[2] - 2 * h, b[3] - 2 * h];
        }
        items.push(it);
      }
      if (!uniform) {
        // Borders on some sides: thin rectangles.
        var x = b[0], y = b[1], w = b[2], hgt = b[3];
        var edges = [[x, y, w, sides[0].w], [x + w - sides[1].w, y, sides[1].w, hgt],
          [x, y + hgt - sides[2].w, w, sides[2].w], [x, y, sides[3].w, hgt]];
        sides.forEach(function (s, i) {
          if (s.w > 0 && s.c) items.push({ kind: 'rect', box: edges[i], fill: s.c });
        });
      }
    }

    function walk(el) {
      if (hidden(el)) return;
      var cs = getComputedStyle(el);
      if (isReplaced(el)) {
        var b = box(el);
        if (b[2] > 0 && b[3] > 0) items.push({ kind: 'image', box: b, cap: cap(el, 'el'), alt: el.getAttribute('alt') || '' });
        return;
      }
      // A masked element is a picture, with what is in it.
      if (el !== slideEl && decorate(el, cs)) return;

      // A list of list items is one text box, one paragraph per item.
      if ((el.tagName === 'UL' || el.tagName === 'OL') && !el.parentElement.closest('li') && listOfItems(el)) {
        list(el);
        return;
      }

      if (cs.display !== 'inline' && hasOwnText(el)) {
        var p = paragraph(el, page, origin);
        if (p) {
          var cb = contentBox(el, origin);
          if (/flex|grid/.test(cs.display)) {
            // The text is an anonymous item beside other items, so its box
            // starts where its text does.
            cb.w = cb.x + cb.w - p.left;
            cb.x = p.left;
          }
          items.push({
            kind: 'text',
            box: [cb.x, p.top, p.pre ? Math.max(cb.w, p.right - cb.x) + 4 : cb.w + 2, p.bottom - p.top],
            noWrap: p.pre || undefined,
            paras: [p.para],
          });
        }
      }
      for (var c = el.firstElementChild; c; c = c.nextElementSibling) {
        if (isReplaced(c) || !isInline(c)) walk(c);
        else walkInline(c);
      }
    }

    // Inline elements hold no blocks, but may hold pictures.
    function walkInline(el) {
      if (hidden(el)) return;
      for (var c = el.firstElementChild; c; c = c.nextElementSibling) {
        if (isReplaced(c) || !isInline(c)) walk(c);
        else walkInline(c);
      }
    }

    function listOfItems(listEl) {
      var lis = listEl.children;
      if (!lis.length) return false;
      for (var i = 0; i < lis.length; i++) {
        if (lis[i].tagName !== 'LI' || getComputedStyle(lis[i]).display !== 'list-item') return false;
      }
      return true;
    }

    function list(listEl) {
      var cb = contentBox(listEl, origin);
      var left = cb.x - 0;
      var paras = [];
      var prevBottom = null, top = null, bottom = null;
      var rightEdge = cb.x + cb.w;
      (function items_(ul, depth) {
        var start = parseInt(ul.getAttribute('start') || '1', 10) || 1;
        for (var li = ul.firstElementChild; li; li = li.nextElementSibling) {
          if (hidden(li)) continue;
          decorate(li, getComputedStyle(li));
          var p = paragraph(li, page, origin);
          var liBox = contentBox(li, origin);
          if (p) {
            var bullet = bulletFor(li);
            p.para.bullet = bullet || undefined;
            if (bullet === '1.') {
              p.para.start = start;
              start = 0;
            }
            p.para.indent = liBox.x - left;
            if (prevBottom !== null) p.para.before = Math.max(0, p.top - prevBottom);
            prevBottom = p.bottom;
            if (top === null) top = p.top;
            bottom = p.bottom;
            paras.push(p.para);
          }
          for (var c = li.firstElementChild; c; c = c.nextElementSibling) {
            if (c.tagName === 'UL' || c.tagName === 'OL') items_(c, depth + 1);
            else if (isReplaced(c) || !isInline(c)) walk(c);
          }
        }
      })(listEl, 0);
      if (paras.length) {
        // Bullets hang in the indent, left of the text.
        var minIndent = Math.min.apply(null, paras.map(function (p) { return p.indent; }));
        var gutter = Math.max(0, 24 - minIndent);
        paras.forEach(function (p) { p.indent += gutter; });
        items.push({ kind: 'text', box: [left - gutter, top, rightEdge - left + gutter + 2, bottom - top], paras: paras });
      }
    }

    walk(slideEl);
    return JSON.stringify(slide);
  };
})();
