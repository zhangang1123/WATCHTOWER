const WS_PROTOCOL = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
const WS_BASE = `${WS_PROTOCOL}//${window.location.host}/ws`
const API_BASE = '/api/v1'

class WebSocketClient {
  constructor() {
    this.ws = null
    this.listeners = {}
    this.reconnectTimer = null
    this.connected = false
  }

  connect() {
    if (this.ws && [WebSocket.OPEN, WebSocket.CONNECTING].includes(this.ws.readyState)) return

    this.ws = new WebSocket(WS_BASE)

    this.ws.onopen = () => {
      this.connected = true
      this.emit('connected', true)
      if (this.reconnectTimer) {
        clearTimeout(this.reconnectTimer)
        this.reconnectTimer = null
      }
    }

    this.ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data)
        this.emit('message', data)
        if (data.type) {
          this.emit(data.type, data)
        }
      } catch (e) {
        console.warn('WS parse error:', e)
      }
    }

    this.ws.onclose = () => {
      this.connected = false
      this.emit('connected', false)
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = setTimeout(() => this.connect(), 3000)
    }

    this.ws.onerror = () => {
      this.ws.close()
    }
  }

  on(event, callback) {
    if (!this.listeners[event]) this.listeners[event] = []
    this.listeners[event].push(callback)
  }

  off(event, callback) {
    if (this.listeners[event]) {
      this.listeners[event] = this.listeners[event].filter(cb => cb !== callback)
    }
  }

  emit(event, data) {
    if (this.listeners[event]) {
      this.listeners[event].forEach(cb => cb(data))
    }
  }

  close() {
    clearTimeout(this.reconnectTimer)
    this.reconnectTimer = null
    if (this.ws) {
      this.ws.onclose = null
      this.ws.close()
      this.ws = null
    }
    this.connected = false
  }
}

const wsClient = new WebSocketClient()

// API methods
export async function fetchIncidents() {
  const res = await fetch(`${API_BASE}/incidents`)
  if (!res.ok) throw new Error(res.statusText)
  return res.json()
}

export async function fetchIncident(id) {
  const res = await fetch(`${API_BASE}/incidents/${id}`)
  if (!res.ok) throw new Error(res.statusText)
  return res.json()
}

export async function triggerAlert(scenario, service, repairOutcome = 'success') {
  const res = await fetch(`${API_BASE}/mock/trigger`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ scenario, service, repair_outcome: repairOutcome }),
  })
  if (!res.ok) throw new Error(res.statusText)
  return res.json()
}

export async function fetchTimeline(id) {
  const res = await fetch(`${API_BASE}/incidents/${id}/timeline`)
  if (!res.ok) throw new Error(res.statusText)
  return res.json()
}

export async function fetchReplay(id) {
  const res = await fetch(`${API_BASE}/incidents/${id}/replay`)
  if (!res.ok) throw new Error(res.statusText)
  return res.json()
}

export async function triggerStorm({ count, duplicateRatio, service }) {
  const res = await fetch(`${API_BASE}/mock/storm`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ count, duplicate_ratio: duplicateRatio, service }),
  })
  if (!res.ok) throw new Error(res.statusText)
  return res.json()
}

export async function fetchHealth() {
  const res = await fetch('/healthz')
  const body = await res.json().catch(() => ({}))
  return { ...body, healthy: res.ok }
}

export { wsClient }
