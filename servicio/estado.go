package main

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Grupos de Gatus que entiende el panel.
const (
	grupoSistemas = "sistemas"
	grupoControl  = "control"
)

type Estado string

const (
	EstadoOK          Estado = "ok"
	EstadoFallando    Estado = "fallando"     // falló la última medición, todavía no alcanza para darlo por caído
	EstadoCaido       Estado = "caido"        // varias fallas seguidas
	EstadoSinDato     Estado = "sin_dato"     // no hay medición reciente o no se puede medir
	EstadoSinInternet Estado = "sin_internet" // solo para el estado general
	EstadoNoMide      Estado = "no_mide"      // solo para el estado general: Gatus no responde o no mide
)

// reglas de lectura de las mediciones.
type reglas struct {
	FallasParaCaido int           // fallas seguidas para pasar de "fallando" a "caído"
	Vigencia        time.Duration // una medición más vieja que esto ya no cuenta
	Historial       int           // cuántas mediciones muestra cada tarjeta
}

var reglasPorDefecto = reglas{
	FallasParaCaido: 3,               // con chequeos cada 60 s: unos 3 minutos
	Vigencia:        3 * time.Minute, // tres intervalos de Gatus
	Historial:       20,
}

// lectura es lo último que el servicio sabe de Gatus.
type lectura struct {
	endpoints   []gatusEndpoint
	gatusFallo  bool // el último pedido a Gatus no funcionó
	leidaAlguna bool // hubo al menos una lectura buena desde que arrancó
}

// Respuesta es el contrato con la página: estados ya resueltos, sin datos
// crudos de Gatus.
type Respuesta struct {
	Version  string    `json:"version"`
	General  General   `json:"general"`
	Internet Estado    `json:"internet"`
	Sistemas []Tarjeta `json:"sistemas"`
}

type General struct {
	Estado  Estado `json:"estado"`
	Titulo  string `json:"titulo"`
	Detalle string `json:"detalle"`
}

type Tarjeta struct {
	Nombre    string `json:"nombre"`
	Estado    Estado `json:"estado"`
	Detalle   string `json:"detalle"`
	HaceS     int    `json:"hace_s"`    // segundos desde la última medición; -1 si no hay
	Historial []bool `json:"historial"` // de la más vieja a la más nueva
}

const nombreNotebook = "Notebook"

// calcular arma la respuesta para la página a partir de la última lectura.
func calcular(l lectura, ahora time.Time, r reglas) Respuesta {
	var sistemas, controles []gatusEndpoint
	ultimaMedicion := time.Time{}
	for _, ep := range l.endpoints {
		ep.Resultados = ordenados(ep.Resultados)
		if n := len(ep.Resultados); n > 0 && ep.Resultados[n-1].Momento.After(ultimaMedicion) {
			ultimaMedicion = ep.Resultados[n-1].Momento
		}
		switch ep.Grupo {
		case grupoSistemas:
			sistemas = append(sistemas, ep)
		case grupoControl:
			controles = append(controles, ep)
		}
	}

	// ¿Gatus está midiendo? Si no, nada de lo demás es confiable.
	mide := !ultimaMedicion.IsZero() && ahora.Sub(ultimaMedicion) <= r.Vigencia
	internet := EstadoSinDato
	if mide {
		internet = estadoInternet(controles, ahora, r)
	}

	// Orden alfabético por nombre, sin depender del orden en que responde Gatus.
	sort.SliceStable(sistemas, func(i, j int) bool {
		return strings.ToLower(sistemas[i].Nombre) < strings.ToLower(sistemas[j].Nombre)
	})
	tarjetas := make([]Tarjeta, 0, len(sistemas)+1)
	for _, ep := range sistemas {
		tarjetas = append(tarjetas, tarjetaSistema(ep, internet, ahora, r))
	}
	tarjetas = append(tarjetas, tarjetaNotebook(l, mide))

	return Respuesta{
		General:  estadoGeneral(l, mide, internet, tarjetas),
		Internet: internet,
		Sistemas: tarjetas,
	}
}

// ordenados devuelve una copia de los resultados de la más vieja a la más
// nueva. Gatus ya los manda así, pero el panel no depende de eso.
func ordenados(rs []gatusResultado) []gatusResultado {
	c := append([]gatusResultado(nil), rs...)
	sort.SliceStable(c, func(i, j int) bool { return c[i].Momento.Before(c[j].Momento) })
	return c
}

// estadoInternet: hay internet si al menos un control reciente funcionó; se
// cortó si todos los controles recientes fallaron.
func estadoInternet(controles []gatusEndpoint, ahora time.Time, r reglas) Estado {
	recientes, fallidos := 0, 0
	for _, c := range controles {
		n := len(c.Resultados)
		if n == 0 {
			continue
		}
		ult := c.Resultados[n-1]
		if ahora.Sub(ult.Momento) > r.Vigencia {
			continue
		}
		recientes++
		if ult.Exito {
			return EstadoOK
		}
		fallidos++
	}
	if recientes > 0 && fallidos == recientes {
		return EstadoCaido
	}
	return EstadoSinDato
}

func tarjetaSistema(ep gatusEndpoint, internet Estado, ahora time.Time, r reglas) Tarjeta {
	t := Tarjeta{Nombre: ep.Nombre, HaceS: -1, Historial: historial(ep.Resultados, r.Historial)}
	n := len(ep.Resultados)
	if n == 0 {
		t.Estado, t.Detalle = EstadoSinDato, "Sin mediciones todavía"
		return t
	}
	ult := ep.Resultados[n-1]
	t.HaceS = int(ahora.Sub(ult.Momento).Seconds())
	if t.HaceS < 0 {
		t.HaceS = 0
	}
	switch {
	case ahora.Sub(ult.Momento) > r.Vigencia:
		t.Estado, t.Detalle = EstadoSinDato, "Medición vieja"
	case internet == EstadoCaido:
		t.Estado, t.Detalle = EstadoSinDato, "No se puede medir sin internet"
	case ult.Exito:
		t.Estado, t.Detalle = EstadoOK, detalleMedicion(ult)
	default:
		t.Estado, t.Detalle = EstadoFallando, detalleMedicion(ult)
		if fallasSeguidas(ep.Resultados) >= r.FallasParaCaido {
			t.Estado = EstadoCaido
		}
	}
	return t
}

func tarjetaNotebook(l lectura, mide bool) Tarjeta {
	t := Tarjeta{Nombre: nombreNotebook, Estado: EstadoOK, Detalle: "Midiendo", HaceS: -1, Historial: []bool{}}
	if !mide {
		// La notebook responde (es la que arma esta respuesta), pero no mide.
		t.Estado = EstadoFallando
		t.Detalle = motivoNoMide(l)
	}
	return t
}

func motivoNoMide(l lectura) string {
	if l.gatusFallo || !l.leidaAlguna {
		return "Gatus no responde"
	}
	return "Gatus no está midiendo"
}

func estadoGeneral(l lectura, mide bool, internet Estado, tarjetas []Tarjeta) General {
	if !mide {
		return General{EstadoNoMide, "La notebook no está midiendo", motivoNoMide(l)}
	}
	if internet == EstadoCaido {
		return General{EstadoSinInternet, "Se cortó internet en casa", "Los sistemas no se pueden medir desde acá"}
	}
	var caidos, fallando []string
	conDato := 0
	for _, t := range tarjetas {
		if t.Nombre == nombreNotebook {
			continue
		}
		switch t.Estado {
		case EstadoCaido:
			caidos = append(caidos, t.Nombre)
		case EstadoFallando:
			fallando = append(fallando, t.Nombre)
		}
		if t.Estado != EstadoSinDato {
			conDato++
		}
	}
	switch {
	case len(caidos) > 0:
		detalle := ""
		if len(fallando) > 0 {
			detalle = "Con fallas: " + strings.Join(fallando, ", ")
		}
		return General{EstadoCaido, "Caído: " + strings.Join(caidos, ", "), detalle}
	case len(fallando) > 0:
		return General{EstadoFallando, "Con fallas: " + strings.Join(fallando, ", "), "Confirmando con las próximas mediciones"}
	case conDato == 0:
		return General{EstadoSinDato, "Esperando mediciones", ""}
	}
	return General{EstadoOK, "Todo funciona", ""}
}

// detalleMedicion describe la última medición sin exponer errores ni
// direcciones: código HTTP y tiempo, o "sin respuesta".
func detalleMedicion(r gatusResultado) string {
	ms := time.Duration(r.Duracion).Milliseconds()
	switch {
	case r.HTTPStatus > 0:
		return fmt.Sprintf("HTTP %d · %d ms", r.HTTPStatus, ms)
	case r.Exito:
		return fmt.Sprintf("%d ms", ms)
	}
	return "Sin respuesta"
}

func fallasSeguidas(rs []gatusResultado) int {
	n := 0
	for i := len(rs) - 1; i >= 0 && !rs[i].Exito; i-- {
		n++
	}
	return n
}

func historial(rs []gatusResultado, max int) []bool {
	if len(rs) > max {
		rs = rs[len(rs)-max:]
	}
	h := make([]bool, len(rs))
	for i, r := range rs {
		h[i] = r.Exito
	}
	return h
}
