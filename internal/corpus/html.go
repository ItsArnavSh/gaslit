package corpus

import (
	"regexp"
	"strings"
	"unicode"
)

// block is a single extracted HTML unit: a paragraph (Heading == 0) or a
// heading (Heading in 1..6).
type block struct {
	Heading int
	Text    string
}

var (
	titleRe   = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	commentRe = regexp.MustCompile(`(?s)<!--.*?-->`)
)

// stripElement removes an element and its contents, case-insensitively.
func stripElement(s, name string) string {
	re := regexp.MustCompile(`(?is)<` + name + `\b[^>]*>.*?</\s*` + name + `\s*>`)
	return re.ReplaceAllString(s, " ")
}

// extractTitle returns the trimmed <title> text, or "".
func extractTitle(s string) string {
	m := titleRe.FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(decodeEntities(stripTags(m[1])))
}

// stripTags removes all tags from a fragment.
func stripTags(s string) string {
	var b strings.Builder
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				b.WriteByte(s[i])
			}
		}
	}
	return b.String()
}

// parseHTML extracts a title and a flat list of heading/paragraph blocks from
// XHTML/HTML bytes. It is deliberately tolerant: it does not require
// well-formed markup and ignores scripts, styles, and document metadata.
func parseHTML(raw []byte) (string, []block) {
	s := string(raw)
	title := extractTitle(s)
	s = commentRe.ReplaceAllString(s, " ")
	s = stripElement(s, "script")
	s = stripElement(s, "style")
	s = stripElement(s, "head")
	s = stripElement(s, "nav")

	var blocks []block
	var buf strings.Builder
	heading := 0

	flush := func() {
		text := normalizeSpace(decodeEntities(buf.String()))
		buf.Reset()
		if text == "" {
			return
		}
		blocks = append(blocks, block{Heading: heading, Text: text})
	}

	for i := 0; i < len(s); {
		if s[i] == '<' {
			end := strings.IndexByte(s[i:], '>')
			if end < 0 {
				break
			}
			tag := s[i+1 : i+end]
			i += end + 1

			name, closing := parseTagName(tag)
			if lvl := headingLevel(name); lvl > 0 {
				if closing {
					flush()
					heading = 0
				} else {
					flush()
					heading = lvl
				}
				continue
			}
			if isBlockTag(name) {
				flush()
			}
			continue
		}
		buf.WriteByte(s[i])
		i++
	}
	flush()
	return title, blocks
}

// parseTagName returns the lowercase element name and whether the tag closes.
func parseTagName(tag string) (string, bool) {
	tag = strings.TrimSpace(tag)
	closing := false
	if strings.HasPrefix(tag, "/") {
		closing = true
		tag = tag[1:]
	}
	end := strings.IndexAny(tag, " \t\r\n/>")
	if end >= 0 {
		tag = tag[:end]
	}
	return strings.ToLower(tag), closing
}

func headingLevel(name string) int {
	if len(name) == 2 && name[0] == 'h' && name[1] >= '1' && name[1] <= '6' {
		return int(name[1] - '0')
	}
	return 0
}

var blockTags = map[string]bool{
	"p": true, "div": true, "br": true, "hr": true, "li": true, "tr": true,
	"td": true, "th": true, "table": true, "ul": true, "ol": true, "dl": true,
	"dt": true, "dd": true, "section": true, "article": true, "blockquote": true,
	"pre": true, "header": true, "footer": true, "figure": true, "figcaption": true,
	"main": true, "aside": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true,
}

func isBlockTag(name string) bool { return blockTags[name] }

// normalizeSpace collapses all whitespace runs (including non-breaking spaces)
// into single spaces and trims the result.
func normalizeSpace(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return unicode.IsSpace(r) || r == '\u00a0'
	}), " ")
}

var namedEntities = map[string]string{
	"amp": "&", "lt": "<", "gt": ">", "quot": "\"", "apos": "'",
	"nbsp": " ", "mdash": "—", "ndash": "–", "horbar": "―",
	"lsquo": "'", "rsquo": "'", "ldquo": "\u201c", "rdquo": "\u201d",
	"hellip": "…", "middot": "·", "bull": "•", "deg": "°",
	"copy": "©", "reg": "®", "trade": "™", "sect": "§", "para": "¶",
	"dag": "†", "ole": "�", "eacute": "é", "egrave": "è", "agrave": "à",
	"ccedil": "ç", "uuml": "ü", "ouml": "ö", "auml": "ä", "szlig": "ß",
	"ntilde": "ñ", "aacute": "á", "iacute": "í", "oacute": "ó", "uacute": "ú",
	"laquo": "«", "raquo": "»", "frac12": "½", "frac14": "¼", "frac34": "¾",
	"times": "×", "divide": "÷", "plusmn": "±", "prime": "′", "Prime": "″",
	"ensp": " ", "emsp": " ", "thinsp": " ", "shy": "", "zwnj": "", "zwj": "",
}

// decodeEntities decodes numeric and the most common named HTML entities,
// dropping unknown ones. It intentionally does not error on malformed input.
func decodeEntities(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '&' {
			b.WriteByte(s[i])
			i++
			continue
		}
		semi := strings.IndexByte(s[i:], ';')
		if semi < 0 || semi > 12 {
			b.WriteByte(s[i])
			i++
			continue
		}
		ent := s[i+1 : i+semi]
		if strings.HasPrefix(ent, "#") {
			if r, ok := decodeNumericEntity(ent[1:]); ok {
				b.WriteRune(r)
				i += semi + 1
				continue
			}
		} else if v, ok := namedEntities[ent]; ok {
			b.WriteString(v)
			i += semi + 1
			continue
		} else if v, ok := namedEntities[strings.ToLower(ent)]; ok {
			b.WriteString(v)
			i += semi + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

func decodeNumericEntity(ent string) (rune, bool) {
	base := 10
	if strings.HasPrefix(ent, "x") || strings.HasPrefix(ent, "X") {
		base = 16
		ent = ent[1:]
	}
	if ent == "" {
		return 0, false
	}
	var v int64
	for _, c := range ent {
		var d int64
		switch {
		case c >= '0' && c <= '9':
			d = int64(c - '0')
		case base == 16 && c >= 'a' && c <= 'f':
			d = int64(c-'a') + 10
		case base == 16 && c >= 'A' && c <= 'F':
			d = int64(c-'A') + 10
		default:
			return 0, false
		}
		v = v*int64(base) + d
		if v > 0x10FFFF {
			return 0, false
		}
	}
	return rune(v), true
}
