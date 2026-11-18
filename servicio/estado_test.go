package main

import (
	"testing"
	"time"
)

var ahoraPrueba = time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC)

// serie arma resultados cada 60 s que terminan `haceUltimo` antes de ahora.
// exitos se lee de más viejo a más nuevo.
func serie(haceUltimo time.Duration, exitos ...bool) []gatusResultado {
	rs := make([]gatusResultado, len(exitos))
	for i, ok := range exitos {
		edad := haceUltimo + time.Duration(len(exitos)-1-i)*time.Minute
		r := gatusResultado{Exito: ok, Momento: ahoraPrueba.Add(-edad), Duracion: int64(180 * time.Millisecond)}
		if ok {
			r.HTTPStatus = 200
		}
		rs[i] = r
	}
	return rs
}

func ep(grupo, nombre string, rs []gatusResultado) gatusEndpoint {
	return gatusEndpoint{Grupo: grupo, Nombre: nombre, Resultados: rs}
}

func controlesOK() []gatusEndpoint {
	return []gatusEndpoint{
		ep(grupoControl, "DNS de Google", serie(10*time.Second, true, true)),
		ep(grupoControl, "DNS de Quad9", serie(20*time.Second, true, true)),
	}
}

func leida(eps ...gatusEndpoint) lectura {
	return lectura{endpoints: eps, leidaAlguna: true}
}

func tarjeta(t *testing.T, r Respuesta, nombre string) Tarjeta {
	t.Helper()
	for _, c := range r.Sistemas {
		if c.Nombre == nombre {
			return c
		}
	}
	t.Fatalf("no hay tarjeta %q en %+v", nombre, r.Sistemas)
	return Tarjeta{}
}

func TestTodoFunciona(t *testing.T) {
	eps := append(controlesOK(),
		ep(grupoSistemas, "VPS", serie(5*time.Second, true, true, true)),
		ep(grupoSistemas, "Bricka", serie(15*time.Second, true, true, true)),
		ep(grupoSistemas, "ivanvallejos.dev", serie(25*time.Second, true, true, true)),
	)
	r := calcular(leida(eps...), ahoraPrueba, reglasPorDefecto)

	if r.General.Estado != EstadoOK || r.General.Titulo != "Todo funciona" {
		t.Fatalf("general = %+v", r.General)
	}
	if r.Internet != EstadoOK {
		t.Fatalf("internet = %s", r.Internet)
	}
	var nombres []string
	for _, c := range r.Sistemas {
		nombres = append(nombres, c.Nombre)
	}
	quiero := []string{"Bricka", "ivanvallejos.dev", "VPS", "Notebook"}
	if len(nombres) != len(quiero) {
		t.Fatalf("tarjetas = %v", nombres)
	}
	for i := range quiero {
		if nombres[i] != quiero[i] {
			t.Fatalf("orden = %v, quiero %v", nombres, quiero)
		}
	}
	b := tarjeta(t, r, "Bricka")
	if b.Detalle != "HTTP 200 · 180 ms" || b.HaceS != 15 || len(b.Historial) != 3 {
		t.Fatalf("bricka = %+v", b)
	}
	if n := tarjeta(t, r, "Notebook"); n.Estado != EstadoOK {
		t.Fatalf("notebook = %+v", n)
	}
}

func TestUnaFallaEsFallandoYTresEsCaido(t *testing.T) {
	eps := append(controlesOK(),
		ep(grupoSistemas, "VPS", serie(5*time.Second, true, true, false)),
		ep(grupoSistemas, "Bricka", serie(5*time.Second, true, false, false, false)),
	)
	r := calcular(leida(eps...), ahoraPrueba, reglasPorDefecto)

	if c := tarjeta(t, r, "VPS"); c.Estado != EstadoFallando || c.Detalle != "Sin respuesta" {
		t.Fatalf("vps = %+v", c)
	}
	if c := tarjeta(t, r, "Bricka"); c.Estado != EstadoCaido {
		t.Fatalf("bricka = %+v", c)
	}
	if r.General.Estado != EstadoCaido || r.General.Titulo != "Caído: Bricka" || r.General.Detalle != "Con fallas: VPS" {
		t.Fatalf("general = %+v", r.General)
	}
}

func TestLentoMuestraCodigoYTiempo(t *testing.T) {
	lento := serie(5*time.Second, true, false)
	lento[1].HTTPStatus = 200
	lento[1].Duracion = int64(3500 * time.Millisecond)
	eps := append(controlesOK(), ep(grupoSistemas, "Bricka", lento))
	r := calcular(leida(eps...), ahoraPrueba, reglasPorDefecto)
	if c := tarjeta(t, r, "Bricka"); c.Estado != EstadoFallando || c.Detalle != "HTTP 200 · 3500 ms" {
		t.Fatalf("bricka = %+v", c)
	}
	if r.General.Estado != EstadoFallando || r.General.Titulo != "Con fallas: Bricka" {
		t.Fatalf("general = %+v", r.General)
	}
}

func TestSinInternetNoDaLosSistemasPorCaidos(t *testing.T) {
	eps := []gatusEndpoint{
		ep(grupoControl, "DNS de Google", serie(10*time.Second, true, false, false)),
		ep(grupoControl, "DNS de Quad9", serie(20*time.Second, true, false)),
		ep(grupoSistemas, "VPS", serie(5*time.Second, true, false, false, false)),
	}
	r := calcular(leida(eps...), ahoraPrueba, reglasPorDefecto)

	if r.Internet != EstadoCaido {
		t.Fatalf("internet = %s", r.Internet)
	}
	if r.General.Estado != EstadoSinInternet {
		t.Fatalf("general = %+v", r.General)
	}
	if c := tarjeta(t, r, "VPS"); c.Estado != EstadoSinDato {
		t.Fatalf("vps = %+v", c)
	}
}

func TestUnControlCaidoNoEsCorteDeInternet(t *testing.T) {
	eps := []gatusEndpoint{
		ep(grupoControl, "DNS de Google", serie(10*time.Second, false, false)),
		ep(grupoControl, "DNS de Quad9", serie(20*time.Second, true, true)),
		ep(grupoSistemas, "VPS", serie(5*time.Second, true)),
	}
	r := calcular(leida(eps...), ahoraPrueba, reglasPorDefecto)
	if r.Internet != EstadoOK || r.General.Estado != EstadoOK {
		t.Fatalf("internet = %s, general = %+v", r.Internet, r.General)
	}
}

func TestMedicionesViejasEsNoMide(t *testing.T) {
	eps := append(
		[]gatusEndpoint{ep(grupoControl, "DNS de Google", serie(5*time.Minute, true))},
		ep(grupoSistemas, "VPS", serie(4*time.Minute, true)),
	)
	r := calcular(leida(eps...), ahoraPrueba, reglasPorDefecto)
	if r.General.Estado != EstadoNoMide || r.General.Detalle != "Gatus no está midiendo" {
		t.Fatalf("general = %+v", r.General)
	}
	if c := tarjeta(t, r, "VPS"); c.Estado != EstadoSinDato {
		t.Fatalf("vps = %+v", c)
	}
	if c := tarjeta(t, r, "Notebook"); c.Estado != EstadoFallando || c.Detalle != "Gatus no está midiendo" {
		t.Fatalf("notebook = %+v", c)
	}
}

func TestGatusNuncaRespondio(t *testing.T) {
	r := calcular(lectura{gatusFallo: true}, ahoraPrueba, reglasPorDefecto)
	if r.General.Estado != EstadoNoMide || r.General.Detalle != "Gatus no responde" {
		t.Fatalf("general = %+v", r.General)
	}
	if len(r.Sistemas) != 1 || r.Sistemas[0].Nombre != "Notebook" {
		t.Fatalf("tarjetas = %+v", r.Sistemas)
	}
}

func TestSinControlesEsperaMediciones(t *testing.T) {
	r := calcular(leida(ep(grupoSistemas, "VPS", nil)), ahoraPrueba, reglasPorDefecto)
	// Hay un sistema sin resultados y ninguna medición: Gatus no midió nada todavía.
	if r.General.Estado != EstadoNoMide {
		t.Fatalf("general = %+v", r.General)
	}
}

func TestResultadosDesordenadosUsaElMasNuevo(t *testing.T) {
	rs := serie(5*time.Second, false, false, true) // el último (más nuevo) es éxito
	rs[0], rs[2] = rs[2], rs[0]                    // Gatus los mandaría al revés
	eps := append(controlesOK(), ep(grupoSistemas, "VPS", rs))
	r := calcular(leida(eps...), ahoraPrueba, reglasPorDefecto)
	c := tarjeta(t, r, "VPS")
	if c.Estado != EstadoOK || c.HaceS != 5 {
		t.Fatalf("vps = %+v", c)
	}
	if c.Historial[0] || !c.Historial[2] {
		t.Fatalf("historial = %v", c.Historial)
	}
}

func TestHistorialRecortado(t *testing.T) {
	exitos := make([]bool, 30)
	for i := range exitos {
		exitos[i] = i%2 == 0
	}
	eps := append(controlesOK(), ep(grupoSistemas, "VPS", serie(5*time.Second, exitos...)))
	r := calcular(leida(eps...), ahoraPrueba, reglasPorDefecto)
	if h := tarjeta(t, r, "VPS").Historial; len(h) != reglasPorDefecto.Historial {
		t.Fatalf("historial de %d", len(h))
	}
}
