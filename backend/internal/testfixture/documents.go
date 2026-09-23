// Package testfixture contains synthetic documents with no personal data.
package testfixture

import (
	"archive/zip"
	"bytes"
	"fmt"
)

func DOCX(extra map[string]string) []byte {
	entries := map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>Built APIs</w:t></w:r></w:p></w:body></w:document>`,
	}
	for k, v := range extra {
		entries[k] = v
	}
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for k, v := range entries {
		w, err := z.Create(k)
		if err != nil {
			panic(err)
		}
		if _, err = w.Write([]byte(v)); err != nil {
			panic(err)
		}
	}
	if err := z.Close(); err != nil {
		panic(err)
	}
	return b.Bytes()
}
func PDF(pages int) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	objects := []string{`<< /Type /Catalog /Pages 2 0 R >>`, ""}
	kids := ""
	for i := 0; i < pages; i++ {
		kids += fmt.Sprintf("%d 0 R ", i+3)
		objects = append(objects, `<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> >>`)
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", pages, kids)
	for i, obj := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return b.Bytes()
}
