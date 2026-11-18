package main

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// app guarda la última lectura de Gatus y la renueva cada tanto.
type app struct {
	gatus   *clienteGatus
	reglas  reglas
	version string
	log     *slog.Logger
	reloj   func() time.Time

	ultima atomic.Pointer[lectura]
}

func nuevaApp(g *clienteGatus, version string, log *slog.Logger) *app {
	a := &app{gatus: g, reglas: reglasPorDefecto, version: version, log: log, reloj: time.Now}
	a.ultima.Store(&lectura{})
	return a
}

// respuesta calcula el estado en el momento del pedido, así las edades
// ("hace 40 s") salen del reloj de la notebook y no del de la tablet.
func (a *app) respuesta() Respuesta {
	r := calcular(*a.ultima.Load(), a.reloj(), a.reglas)
	r.Version = a.version
	return r
}

// leer pide los estados a Gatus una vez. Si falla, conserva los datos
// anteriores: si la falla sigue, envejecen y el panel pasa a "no está midiendo".
func (a *app) leer(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	anterior := a.ultima.Load()
	endpoints, err := a.gatus.estados(ctx, a.reglas.Historial)
	if err != nil {
		if !anterior.gatusFallo {
			a.log.Warn("no se pudo leer Gatus", "error", err)
		}
		siguiente := *anterior
		siguiente.gatusFallo = true
		a.ultima.Store(&siguiente)
		return
	}
	if anterior.gatusFallo {
		a.log.Info("Gatus responde de nuevo")
	}
	a.ultima.Store(&lectura{endpoints: endpoints, leidaAlguna: true})
}

// leerCadaTanto lee Gatus al arrancar y después cada `cada`.
func (a *app) leerCadaTanto(ctx context.Context, cada time.Duration) {
	a.leer(ctx)
	t := time.NewTicker(cada)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.leer(ctx)
		}
	}
}
