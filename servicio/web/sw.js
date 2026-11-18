'use strict';

// Guarda la página (no los datos) para que, si la app se recarga con la
// notebook caída, muestre "Sin conexión con la notebook" en vez del error de
// Firefox. Siempre intenta la red primero: con la notebook disponible llega la
// versión nueva.

const CACHE = 'argos';
const PAGINA = ['/', '/estilos.css', '/panel.js', '/manifest.webmanifest', '/iconos/192.png'];
const ESPERA_RED_MS = 5000;

self.addEventListener('install', (evento) => {
  evento.waitUntil(
    caches
      .open(CACHE)
      .then((cache) => cache.addAll(PAGINA))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener('activate', (evento) => {
  evento.waitUntil(self.clients.claim());
});

self.addEventListener('fetch', (evento) => {
  const pedido = evento.request;
  const url = new URL(pedido.url);
  // Los datos nunca salen de la copia guardada.
  if (pedido.method !== 'GET' || url.origin !== self.location.origin || url.pathname.startsWith('/api/')) {
    return;
  }
  evento.respondWith(redPrimero(pedido));
});

// El corte va con una carrera y no con AbortController: un pedido de
// navegación no se puede rearmar con opciones nuevas.
function vencer(ms) {
  return new Promise((_, rechazar) => setTimeout(() => rechazar(new Error('sin respuesta')), ms));
}

async function redPrimero(pedido) {
  try {
    const resp = await Promise.race([fetch(pedido), vencer(ESPERA_RED_MS)]);
    if (resp.ok) {
      const copia = resp.clone();
      caches.open(CACHE).then((cache) => cache.put(pedido, copia));
    }
    return resp;
  } catch (_) {
    const guardada = await caches.match(pedido, { ignoreSearch: true });
    return guardada || Response.error();
  }
}
