// Argos: lee Gatus, resuelve los estados y sirve la página del panel para la
// tablet. Escucha solo en la notebook; `tailscale serve` lo publica por HTTPS
// dentro de la tailnet.
package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

const cadaLecturaGatus = 30 * time.Second

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := correr(log); err != nil {
		log.Error("argos se detuvo", "error", err)
		os.Exit(1)
	}
}

func correr(log *slog.Logger) error {
	escucha := entorno("ARGOS_ESCUCHA", "127.0.0.1:8081")
	gatusURL := entorno("ARGOS_GATUS_URL", "http://127.0.0.1:8080")

	web, err := fs.Sub(webEmbebida, "web")
	if err != nil {
		return err
	}
	version, err := versionDe(web)
	if err != nil {
		return err
	}

	ctx, parar := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer parar()

	a := nuevaApp(nuevoClienteGatus(gatusURL), version, log)
	go a.leerCadaTanto(ctx, cadaLecturaGatus)

	srv := &http.Server{
		Addr:              escucha,
		Handler:           nuevoServidor(a, web),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	errServidor := make(chan error, 1)
	go func() { errServidor <- srv.ListenAndServe() }()
	log.Info("argos escuchando", "direccion", escucha, "gatus", gatusURL, "version", version)

	select {
	case err := <-errServidor:
		return err
	case <-ctx.Done():
	}
	apagar, cancelar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelar()
	if err := srv.Shutdown(apagar); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func entorno(clave, porDefecto string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return porDefecto
}
