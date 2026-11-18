package main

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"mime"
	"net/http"
	"strings"
)

//go:embed web
var webEmbebida embed.FS

func init() {
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

// Todo sale de la notebook: la política de seguridad del navegador bloquea
// cualquier recurso externo aunque alguien lo agregue por error a la página.
const politicaContenido = "default-src 'self'; img-src 'self'; style-src 'self'; script-src 'self'; " +
	"connect-src 'self'; manifest-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

func nuevoServidor(a *app, web fs.FS) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/estado", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(a.respuesta())
	})
	archivos := http.FileServerFS(web)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// Sin listados de carpetas.
		if r.URL.Path != "/" && strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		archivos.ServeHTTP(w, r)
	})
	return conCabeceras(mux)
}

func conCabeceras(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", politicaContenido)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		h.ServeHTTP(w, r)
	})
}

// versionDe resume los archivos de la página. Si cambia (porque se desplegó
// una versión nueva), la página se recarga sola sin tocar la tablet.
func versionDe(web fs.FS) (string, error) {
	h := sha256.New()
	err := fs.WalkDir(web, ".", func(ruta string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		datos, err := fs.ReadFile(web, ruta)
		if err != nil {
			return err
		}
		h.Write([]byte(ruta))
		h.Write(datos)
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}
