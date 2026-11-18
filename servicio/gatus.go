package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// Solo los campos de la API de Gatus que usa el panel.
// Formato: GET /api/v1/endpoints/statuses (Gatus v5).

type gatusResultado struct {
	HTTPStatus int       `json:"status"`   // 0 si no hubo respuesta HTTP (ping, DNS, sin conexión)
	Duracion   int64     `json:"duration"` // nanosegundos
	Exito      bool      `json:"success"`
	Momento    time.Time `json:"timestamp"`
	// Gatus también manda "errors" con hostnames e IPs: a propósito no se leen,
	// para que nunca lleguen a la tablet.
}

type gatusEndpoint struct {
	Nombre     string           `json:"name"`
	Grupo      string           `json:"group"`
	Resultados []gatusResultado `json:"results"`
}

type clienteGatus struct {
	base string
	http *http.Client
}

func nuevoClienteGatus(base string) *clienteGatus {
	return &clienteGatus{base: base, http: &http.Client{Timeout: 5 * time.Second}}
}

// estados pide los últimos `porEndpoint` resultados de cada chequeo.
func (c *clienteGatus) estados(ctx context.Context, porEndpoint int) ([]gatusEndpoint, error) {
	url := c.base + "/api/v1/endpoints/statuses?page=1&pageSize=" + strconv.Itoa(porEndpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gatus respondió %d", resp.StatusCode)
	}
	var endpoints []gatusEndpoint
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&endpoints); err != nil {
		return nil, fmt.Errorf("json de gatus: %w", err)
	}
	return endpoints, nil
}
