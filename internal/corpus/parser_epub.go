package corpus

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strings"
)

const containerPath = "META-INF/container.xml"

type containerXML struct {
	Rootfiles []struct {
		FullPath  string `xml:"full-path,attr"`
		MediaType string `xml:"media-type,attr"`
	} `xml:"rootfiles>rootfile"`
}

type opfXML struct {
	Metadata struct {
		Title string `xml:"title"`
	} `xml:"metadata"`
	Manifest struct {
		Items []struct {
			ID         string `xml:"id,attr"`
			Href       string `xml:"href,attr"`
			MediaType  string `xml:"media-type,attr"`
			Properties string `xml:"properties,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		Refs []struct {
			IDRef  string `xml:"idref,attr"`
			Linear string `xml:"linear,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

// parseEPUB parses an EPUB (zip container of XHTML documents) into a Source,
// following the OPF spine for reading order.
func parseEPUB(name, origin string, data []byte) (*Source, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("corpus: open EPUB: %w", err)
	}

	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[cleanZipPath(f.Name)] = f
	}

	docs, title, err := epubSpineDocs(zr, files)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("corpus: EPUB contains no readable XHTML documents")
	}

	var blocks []block
	for _, d := range docs {
		raw, err := readZipFile(files[d])
		if err != nil {
			continue
		}
		_, bs := parseHTML(raw)
		blocks = append(blocks, bs...)
	}
	blocks = trimGutenbergBlocks(blocks)

	chapters := BuildChapters(blocks, DefaultChapterOptions())
	if title == "" {
		title = name
	}
	return &Source{Name: name, Origin: origin, Format: "epub", Title: title, Chapters: chapters}, nil
}

// epubSpineDocs returns the ordered list of XHTML document paths from the OPF
// spine, plus the package title. It falls back to all HTML documents in the zip
// when no usable OPF is present.
func epubSpineDocs(zr *zip.Reader, files map[string]*zip.File) ([]string, string, error) {
	container, err := readZipFile(files[cleanZipPath(containerPath)])
	if err != nil {
		return fallbackDocs(zr), "", nil
	}
	var c containerXML
	if err := xml.Unmarshal(container, &c); err != nil || len(c.Rootfiles) == 0 {
		return fallbackDocs(zr), "", nil
	}
	opfPath := cleanZipPath(c.Rootfiles[0].FullPath)
	opfRaw, err := readZipFile(files[opfPath])
	if err != nil {
		return fallbackDocs(zr), "", nil
	}
	var opf opfXML
	if err := xml.Unmarshal(opfRaw, &opf); err != nil {
		return fallbackDocs(zr), "", nil
	}

	byID := make(map[string]string, len(opf.Manifest.Items))
	for _, it := range opf.Manifest.Items {
		byID[it.ID] = it.Href
	}

	dir := path.Dir(opfPath)
	var docs []string
	for _, ref := range opf.Spine.Refs {
		if strings.EqualFold(ref.Linear, "no") {
			continue
		}
		href, ok := byID[ref.IDRef]
		if !ok {
			continue
		}
		p := resolveHref(dir, href)
		if _, ok := files[p]; ok {
			docs = append(docs, p)
		}
	}
	if len(docs) == 0 {
		return fallbackDocs(zr), normalizeSpace(opf.Metadata.Title), nil
	}
	return docs, normalizeSpace(opf.Metadata.Title), nil
}

// fallbackDocs returns every HTML/XHTML document in the zip, sorted by name.
func fallbackDocs(zr *zip.Reader) []string {
	var docs []string
	for _, f := range zr.File {
		name := cleanZipPath(f.Name)
		if isHTMLDoc(name) && !isAuxiliaryDoc(name) {
			docs = append(docs, name)
		}
	}
	sort.Strings(docs)
	return docs
}

func isHTMLDoc(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".xhtml") || strings.HasSuffix(lower, ".html") || strings.HasSuffix(lower, ".htm")
}

func isAuxiliaryDoc(name string) bool {
	base := strings.ToLower(path.Base(name))
	switch base {
	case "nav.xhtml", "nav.html", "toc.xhtml", "toc.html", "cover.xhtml", "cover.html", "titlepage.xhtml":
		return true
	}
	return false
}

// resolveHref joins a relative manifest href to the OPF directory.
func resolveHref(dir, href string) string {
	if u, err := url.PathUnescape(href); err == nil {
		href = u
	}
	if strings.HasPrefix(href, "/") {
		return cleanZipPath(href)
	}
	return cleanZipPath(path.Join(dir, href))
}

func cleanZipPath(p string) string {
	return strings.TrimPrefix(path.Clean("/"+strings.ReplaceAll(p, "\\", "/")), "/")
}

func readZipFile(f *zip.File) ([]byte, error) {
	if f == nil {
		return nil, fmt.Errorf("corpus: zip entry not found")
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}
