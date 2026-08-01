import net from 'node:net'

const [mode, ...rawPorts] = process.argv.slice(2)
const ports = rawPorts.map(Number).filter(Number.isInteger)

if (!['free', 'wait'].includes(mode) || ports.length === 0) {
  console.error('Usage: node scripts/port-check.mjs <free|wait> <port...>')
  process.exit(2)
}

function canBind(port) {
  return new Promise((resolve) => {
    const server = net.createServer()
    server.unref()
    server.once('error', () => resolve(false))
    server.listen({ host: '0.0.0.0', port, exclusive: true }, () => {
      server.close(() => resolve(true))
    })
  })
}

function canConnect(port) {
  return new Promise((resolve) => {
    const socket = net.createConnection({ host: '127.0.0.1', port })
    socket.setTimeout(500)
    socket.once('connect', () => {
      socket.destroy()
      resolve(true)
    })
    socket.once('timeout', () => {
      socket.destroy()
      resolve(false)
    })
    socket.once('error', () => resolve(false))
  })
}

if (mode === 'free') {
  const occupied = []
  for (const port of ports) {
    if (!(await canBind(port))) occupied.push(port)
  }
  if (occupied.length) {
    console.error(`Ports already in use: ${occupied.join(', ')}`)
    process.exit(1)
  }
  process.exit(0)
}

const deadline = Date.now() + 20_000
const pending = new Set(ports)
while (pending.size && Date.now() < deadline) {
  for (const port of pending) {
    if (await canConnect(port)) pending.delete(port)
  }
  if (pending.size) await new Promise((resolve) => setTimeout(resolve, 200))
}

if (pending.size) {
  console.error(`Timed out waiting for ports: ${[...pending].join(', ')}`)
  process.exit(1)
}
