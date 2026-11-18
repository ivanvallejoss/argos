'use strict';

(() => {
  const CADA_MS = 30000; // cada cuánto pide el estado
  const ESPERA_MAX_MS = 10000; // corte de un pedido que no vuelve
  const FALLAS_PARA_SIN_CONEXION = 2; // pedidos fallidos seguidos para decir "sin conexión"
  const INACTIVIDAD_MS = 150000; // las animaciones constantes corren hasta 2,5 min después de un toque
  const CASILLAS_HISTORIAL = 20;

  // Modo medición para la prueba de batería: ?medir=quieto o ?medir=animado
  const modoMedicion = new URLSearchParams(location.search).get('medir');

  const TEXTO_ESTADO = {
    ok: 'Funciona',
    fallando: 'Con fallas',
    caido: 'Caído',
    sin_dato: 'Sin dato',
  };

  const $ = (id) => document.getElementById(id);
  const general = $('general');
  const titulo = $('titulo');
  const detalle = $('detalle');
  const contenedor = $('sistemas');
  const actualizado = $('actualizado');

  let version = null;
  let fallas = 0;
  let temporizador = 0;
  let ultimoExito = null;
  let firmaTarjetas = '';
  const tarjetas = new Map();

  // Solo toca el DOM si algo cambió: menos trabajo de dibujado.
  function poner(el, texto) {
    if (el.textContent !== texto) el.textContent = texto;
  }

  function ponerEstado(el, estado) {
    if (el.dataset.estado !== estado) el.dataset.estado = estado;
  }

  function hora(fecha) {
    return fecha.toLocaleTimeString('es-AR', { hour: '2-digit', minute: '2-digit', hour12: false });
  }

  function hace(segundos) {
    if (segundos < 0) return '';
    if (segundos < 60) return 'hace menos de 1 min';
    const minutos = Math.floor(segundos / 60);
    if (minutos < 60) return `hace ${minutos} min`;
    return `hace ${Math.floor(minutos / 60)} h`;
  }

  function nodo(etiqueta, clase, texto = '') {
    const el = document.createElement(etiqueta);
    el.className = clase;
    el.textContent = texto;
    return el;
  }

  function crearTarjeta(nombre) {
    const t = {
      raiz: nodo('article', 'tarjeta'),
      estado: nodo('p', 'estado'),
      detalle: nodo('p', 'detalle'),
      hace: nodo('p', 'hace'),
      historial: nodo('div', 'historial'),
      ultimoEstado: null,
    };
    for (let i = 0; i < CASILLAS_HISTORIAL; i++) t.historial.appendChild(document.createElement('i'));
    t.raiz.append(nodo('h2', 'nombre', nombre), t.estado, t.detalle, t.hace, t.historial);
    t.estado.addEventListener('animationend', () => t.estado.classList.remove('avisando'));
    return t;
  }

  // Rearma las tarjetas solo si cambió la lista de sistemas.
  function rearmar(sistemas) {
    const nuevas = new Map();
    const fragmento = document.createDocumentFragment();
    for (const s of sistemas) {
      const t = tarjetas.get(s.nombre) || crearTarjeta(s.nombre);
      nuevas.set(s.nombre, t);
      fragmento.appendChild(t.raiz);
    }
    contenedor.replaceChildren(fragmento);
    tarjetas.clear();
    nuevas.forEach((t, nombre) => tarjetas.set(nombre, t));
  }

  function pintarHistorial(contenedorHistorial, lista) {
    const casillas = contenedorHistorial.children;
    const desde = CASILLAS_HISTORIAL - lista.length; // las más nuevas quedan a la derecha
    for (let i = 0; i < CASILLAS_HISTORIAL; i++) {
      const j = i - desde;
      const clase = j < 0 ? '' : lista[j] ? 'ok' : 'falla';
      if (casillas[i].className !== clase) casillas[i].className = clase;
    }
  }

  function pintarTarjeta(t, s) {
    // Aviso por cambio de estado: siempre activo, haya o no alguien mirando.
    if (t.ultimoEstado !== null && t.ultimoEstado !== s.estado && s.estado === 'caido') {
      t.estado.classList.add('avisando');
    }
    t.ultimoEstado = s.estado;
    ponerEstado(t.raiz, s.estado);
    poner(t.estado, TEXTO_ESTADO[s.estado] || s.estado);
    poner(t.detalle, s.detalle);
    poner(t.hace, hace(s.hace_s));
    t.historial.hidden = s.historial.length === 0; // la notebook no tiene historial propio
    pintarHistorial(t.historial, s.historial.slice(-CASILLAS_HISTORIAL));
  }

  function pintar(datos) {
    document.body.classList.remove('desconectado');
    ponerEstado(general, datos.general.estado);
    poner(titulo, datos.general.titulo);
    poner(detalle, datos.general.detalle);

    const firma = datos.sistemas.map((s) => s.nombre).join('\n');
    if (firma !== firmaTarjetas) {
      rearmar(datos.sistemas);
      firmaTarjetas = firma;
    }
    for (const s of datos.sistemas) pintarTarjeta(tarjetas.get(s.nombre), s);

    ultimoExito = new Date();
    poner(actualizado, `Actualizado ${hora(ultimoExito)}`);
  }

  function pintarSinConexion() {
    document.body.classList.add('desconectado');
    ponerEstado(general, 'sin_conexion');
    poner(titulo, 'Sin conexión con la notebook');
    poner(detalle, ultimoExito ? `Último dato: ${hora(ultimoExito)}` : 'Todavía no llegó ningún dato');
  }

  async function actualizar() {
    clearTimeout(temporizador);
    const control = new AbortController();
    const corte = setTimeout(() => control.abort(), ESPERA_MAX_MS);
    try {
      const resp = await fetch('/api/estado', { cache: 'no-store', signal: control.signal });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const datos = await resp.json();
      // Versión nueva desplegada en la notebook: recargar sin tocar la tablet.
      if (version === null) version = datos.version;
      else if (datos.version !== version) {
        location.reload();
        return;
      }
      fallas = 0;
      pintar(datos);
    } catch (_) {
      fallas += 1;
      if (fallas >= FALLAS_PARA_SIN_CONEXION || ultimoExito === null) pintarSinConexion();
    } finally {
      clearTimeout(corte);
      if (!document.hidden) temporizador = setTimeout(actualizar, CADA_MS);
    }
  }

  // --- Animaciones constantes: solo un rato después de un toque ---------------

  let temporizadorInactividad = 0;

  function marcarActividad() {
    document.body.classList.add('activo');
    clearTimeout(temporizadorInactividad);
    temporizadorInactividad = setTimeout(() => document.body.classList.remove('activo'), INACTIVIDAD_MS);
  }

  if (modoMedicion === 'quieto') {
    document.body.classList.add('quieto');
    poner($('modo'), 'Medición: sin animaciones');
  } else if (modoMedicion === 'animado') {
    document.body.classList.add('activo');
    poner($('modo'), 'Medición: animación constante');
  } else {
    document.addEventListener('pointerdown', marcarActividad, { passive: true });
    marcarActividad(); // abrir la app ya es un toque
  }

  // Con la pantalla apagada o la app en segundo plano no se pide nada.
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) clearTimeout(temporizador);
    else actualizar();
  });

  // Guarda la página para poder mostrar "sin conexión" aunque se recargue
  // con la notebook caída.
  if ('serviceWorker' in navigator) {
    navigator.serviceWorker.register('/sw.js').catch(() => {});
  }

  actualizar();
})();
