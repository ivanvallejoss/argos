package main

import (
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Cuerpo copiado de las pruebas de la API de Gatus v5.38.0
// (api/endpoint_status_test.go): es el formato real que devuelve.
const cuerpoGatusReal = `[{"name":"name","group":"group","key":"group_name","results":[{"status":200,"hostname":"example.org","duration":150000000,"conditionResults":[{"condition":"[STATUS] == 200","success":true},{"condition":"[RESPONSE_TIME] < 500","success":true},{"condition":"[CERTIFICATE_EXPIRATION] < 72h","success":true}],"success":true,"timestamp":"0001-01-01T00:00:00Z"},{"status":200,"hostname":"example.org","duration":750000000,"errors":["error-1","error-2"],"conditionResults":[{"condition":"[STATUS] == 200","success":true},{"condition":"[RESPONSE_TIME] < 500","success":false},{"condition":"[CERTIFICATE_EXPIRATION] < 72h","success":false}],"success":false,"timestamp":"0001-01-01T00:00:00Z"}]}]`

func TestLeeElFormatoRealDeGatus(t *testing.T) {
	var pedido string
	gatus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pedido = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, cuerpoGatusReal)
	}))
	defer gatus.Close()

	eps, err := nuevoClienteGatus(gatus.URL).estados(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if pedido != "/api/v1/endpoints/statuses?page=1&pageSize=20" {
		t.Fatalf("pedido = %s", pedido)
	}
	if len(eps) != 1 || eps[0].Grupo != "group" || eps[0].Nombre != "name" || len(eps[0].Resultados) != 2 {
		t.Fatalf("endpoints = %+v", eps)
	}
	r := eps[0].Resultados[1]
	if r.HTTPStatus != 200 || r.Exito || time.Duration(r.Duracion) != 750*time.Millisecond {
		t.Fatalf("resultado = %+v", r)
	}
}

func servidorDePrueba(t *testing.T, gatusURL string) (*httptest.Server, *app) {
	t.Helper()
	web, err := fs.Sub(webEmbebida, "web")
	if err != nil {
		t.Fatal(err)
	}
	a := nuevaApp(nuevoClienteGatus(gatusURL), "prueba", slog.New(slog.NewTextHandler(io.Discard, nil)))
	s := httptest.NewServer(nuevoServidor(a, web))
	t.Cleanup(s.Close)
	return s, a
}

func TestApiEstadoNoExponeErroresDeGatus(t *testing.T) {
	ahora := time.Now().UTC().Format(time.RFC3339)
	cuerpo := `[
	 {"name":"DNS de Google","group":"control","key":"control_dns-de-google","results":[{"duration":20000000,"success":true,"timestamp":"` + ahora + `"}]},
	 {"name":"VPS","group":"sistemas","key":"sistemas_vps","results":[{"duration":0,"errors":["no se pudo hacer ping a 203.0.113.7"],"success":false,"timestamp":"` + ahora + `"}]}
	]`
	gatus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, cuerpo)
	}))
	defer gatus.Close()
	s, a := servidorDePrueba(t, gatus.URL)
	a.leer(context.Background())

	resp, err := http.Get(s.URL + "/api/estado")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	crudo, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(crudo), "203.0.113.7") {
		t.Fatalf("la respuesta expone la IP: %s", crudo)
	}
	if resp.Header.Get("Cache-Control") != "no-store" || resp.Header.Get("Content-Security-Policy") == "" {
		t.Fatalf("cabeceras = %v", resp.Header)
	}
	var r Respuesta
	if err := json.Unmarshal(crudo, &r); err != nil {
		t.Fatal(err)
	}
	if r.Version != "prueba" || r.General.Estado != EstadoFallando || r.Internet != EstadoOK {
		t.Fatalf("respuesta = %+v", r)
	}
}

func TestGatusCaidoConservaYLuegoNoMide(t *testing.T) {
	s, a := servidorDePrueba(t, "http://127.0.0.1:1") // nadie escucha ahí
	a.leer(context.Background())
	resp, err := http.Get(s.URL + "/api/estado")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var r Respuesta
	_ = json.NewDecoder(resp.Body).Decode(&r)
	if r.General.Estado != EstadoNoMide || r.General.Detalle != "Gatus no responde" {
		t.Fatalf("general = %+v", r.General)
	}
}

func TestArchivosDeLaPagina(t *testing.T) {
	s, _ := servidorDePrueba(t, "http://127.0.0.1:1")
	casos := []struct {
		ruta, metodo string
		codigo       int
		tipo         string
	}{
		{"/", "GET", 200, "text/html"},
		{"/panel.js", "GET", 200, "javascript"},
		{"/estilos.css", "GET", 200, "text/css"},
		{"/manifest.webmanifest", "GET", 200, "application/manifest+json"},
		{"/iconos/192.png", "GET", 200, "image/png"},
		{"/iconos/", "GET", 404, ""},
		{"/api/estado", "POST", 405, ""},
		{"/no-existe", "GET", 404, ""},
	}
	for _, c := range casos {
		req, _ := http.NewRequest(c.metodo, s.URL+c.ruta, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != c.codigo {
			t.Errorf("%s %s = %d, quiero %d", c.metodo, c.ruta, resp.StatusCode, c.codigo)
		}
		if c.tipo != "" && !strings.Contains(resp.Header.Get("Content-Type"), c.tipo) {
			t.Errorf("%s: tipo %q", c.ruta, resp.Header.Get("Content-Type"))
		}
	}
}
