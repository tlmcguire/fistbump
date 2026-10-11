# Phase 6: PDF export

Build on `main` after [phase 5](phase05.md) merges, from [api.md](api.md) ("Tailored resumes", "Resume import and generation") and [architecture.md](architecture.md) ("Security model"). Can run in parallel with [phase 7](phase07.md). Overview and schedule: [roadmap.md](roadmap.md).

## Goal

The user exports the master resume or a tailored resume as a PDF and saves it where they choose. If PDF generation fails, the app offers Markdown instead. This meets FR-4.

Go returns bytes and never receives a file path. Electron shows the save dialog and writes the file.

## Scope

In:

- The `pdf` package: the Markdown from `render.Master` and `render.Apply` to a one-column resume PDF, with an embedded UTF-8 font.
- `POST /v1/tailored-resumes/{id}/export` and `POST /v1/resume/export`.
- Binary responses through `backend.js`, the native save dialog, and Export buttons in the Resume and Tailor views.

Out: recording the saved path on an application (phase 7 does it once applications exist), resume templates or themes (after the MVP), DOCX export (not planned).

## Routes

Both as written in [api.md](api.md).

```
POST /v1/tailored-resumes/7/export
{ "format": "pdf" }

200
Content-Type: application/pdf
Content-Disposition: attachment; filename="Ada Lovelace - Acme - Data Analyst.pdf"
<bytes>
```

- `format` is `pdf` or `md`. `md` returns the stored text with `Content-Type: text/markdown; charset=utf-8`.
- **Filename.** Tailored: `<full name> - <company> - <position>.<ext>`; master: `<full name> - Resume.<ext>`. Remove characters not allowed in file names on any platform (`/\:*?"<>|` and control characters), collapse spaces, cap at 120 characters. Use RFC 6266's `filename*=UTF-8''...` as well, for names outside ASCII.
- **Failure.** If PDF generation fails, return `500 internal` with `details.fallback: "md"` so the UI can offer Markdown without guessing.

| Status | `code` | When |
|--------|--------|------|
| 404 | `not_found` | Unknown tailored resume; master export with no profile |
| 422 | `validation_failed` | `format` not `pdf` or `md` |
| 500 | `internal` | PDF generation failed (`details.fallback: "md"`) |

## Layout

One column, US Letter, 0.6 inch margins. The PDF follows the Markdown structure that `render.Master` writes:

| Markdown | PDF |
|----------|-----|
| `# Name` | Name, 20 pt bold |
| Line after the name | Contact line, 9.5 pt, separated by ` · `; links (anything that looks like a URL or email) are clickable |
| `## Section` | Section title, 11 pt bold, small caps or upper case, with a thin rule below |
| `### Title, Company` | Entry title, 10.5 pt bold |
| Line after an entry title with ` \| ` | Meta line (dates, location), 9.5 pt, gray, right after the title |
| `- item` | Bullet with a hanging indent |
| Other paragraphs | Body text, 10 pt, 1.25 line height |

- Keep an entry's title and meta line on the same page as its first body line (check the remaining height before drawing a title).
- Page numbers only when there is more than one page.
- Text is drawn as written. Inline Markdown (`**bold**`, links in `[text](url)` form) is not produced by `render`, so the `pdf` package only needs to undo `render`'s line-start escapes.

## Files, in build order

Progress:

- [ ] 1. `pdf/fonts/`
- [ ] 2. `pdf.go`
- [ ] 3. `export.go` (api)
- [ ] 4. `backend.js` binary responses
- [ ] 5. `ipc-handlers.js` save dialog, `preload.js`
- [ ] 6. Export buttons
- [ ] Tests and tooling

### Backend

#### 1. `backend/internal/pdf/fonts/`

**Goal:** a font that prints every character a name or job may contain.

- The full build found that fpdf's built-in fonts are Latin-1 only: "Łukasiewicz" printed with `?`. Embed a TTF with wide Latin coverage under the SIL Open Font License: Inter (Regular, Bold), as the full build used. Include `OFL.txt` beside it.
- Embed with `go:embed`. Inter adds about 1 MB to the binary; check against NFR-5 in phase 9.

References: [Inter](https://rsms.me/inter/), [SIL Open Font License](https://openfontlicense.org/).

#### 2. `backend/internal/pdf/pdf.go`

**Goal:** `Render(markdown string) ([]byte, error)`.

- Use `github.com/go-pdf/fpdf` (the maintained fork of gofpdf, MIT, pure Go). Load the fonts with `AddUTF8FontFromBytes`.
- Walk the Markdown line by line with the mapping in "Layout". No Markdown library is needed for this subset.
- Links: `LinkString` over the drawn text for URLs (add `https://` when missing) and `mailto:` for emails.
- Return fpdf's accumulated error (`pdf.Error()`) instead of panicking; recover any panic from the library and return it as an error, so the API can offer Markdown.
- Set the document title (`SetTitle`) to the filename without extension, and the creator to "FistBump".

References: [go-pdf/fpdf](https://pkg.go.dev/github.com/go-pdf/fpdf).

#### 3. `backend/internal/api/export.go`

**Goal:** the two export routes.

- Tailored: load, then `pdf.Render(content)` or return `content` for `md`.
- Master: `render.Master(store.GetMaster)` then the same.
- Write the bytes with the headers above. Do not go through `writeJSON`.

### Electron

#### 4. `app/main/backend.js`

**Goal:** requests that return bytes.

- Add `requestRaw(method, path, body)`: returns `{ status, headers, bytes }` (an `ArrayBuffer` from `fetch`) for success, and the parsed error shape when the response is JSON.

#### 5. `app/main/ipc-handlers.js` and `app/preload/preload.js`

**Goal:** the export flow, owned by the main process.

- Channels `resume:export` (format) and `tailored:export` (id, format). Each: call `requestRaw`; read the suggested filename from `Content-Disposition`; show `dialog.showSaveDialog` with that name and a filter for the format, starting in the user's Documents folder; write with `fs.promises.writeFile`; return `{ saved: true, path }`, `{ saved: false }` if the user canceled, or the error shape.
- On `500` with `details.fallback: "md"`, return the error; the renderer asks the user and calls again with `md`.
- The renderer never sends a path, and main never writes anywhere the user did not pick in the dialog.
- Preload: `api.resume.export(format)`, `api.tailored.export(id, format)`.

References: [dialog.showSaveDialog](https://www.electronjs.org/docs/latest/api/dialog#dialogshowsavedialogwindow-options).

### Renderer

#### 6. `resume-builder.js` and `diff-view.js`

- Resume view: "Export PDF" next to Preview.
- Tailor view: "Export PDF" on a built tailored resume.
- After saving, show the path with a "Show in folder" action (a new channel `shell:showItem` calling `shell.showItemInFolder`; main checks the path is the one it just wrote).
- On a PDF failure, show "PDF export failed. Save as Markdown instead?" and call again with `md`.

### Tests and tooling

- `pdf_test.go`:
  - Output starts with `%PDF-` and ends with `%%EOF`.
  - A long resume (three pages of experience) produces more than one page and no error.
  - Names and text with `Ł`, `é`, `ñ`, `ü`, and curly quotes render without error. Whether they print correctly is checked by eye in the manual step: reading text back needs a PDF reader library, and the one the full build used garbled UTF-8 text until patched.
  - Empty sections and a resume with only a name do not fail.
- `api_test.go`: both routes in both formats, headers and filename sanitizing (a company with `/` and `:`), `404`, `422`.
- Manual: open exported PDFs in Preview (macOS), Edge or Acrobat (Windows) and a Linux viewer; click a link; select and copy text.

## Done when

- The master resume and a tailored resume export as PDFs that open in each platform's viewer, with accented names printed correctly and links clickable.
- Canceling the save dialog saves nothing and shows no error.
- A forced PDF failure (for example a test-only font error) offers Markdown, and the Markdown saves.
- `go test ./...` and `npm test` pass.
