<script setup>
import { computed, onMounted, onUnmounted, ref } from 'vue'

const apiBase = import.meta.env.VITE_API_BASE || 'http://localhost:8080'
const showId = ref('show-1')
const movie = ref('Demo Movie')
const showDate = ref(new Date().toISOString().slice(0, 10))
const userId = ref('user-1')
const role = ref('USER')
const token = ref('')
const seats = ref([])
const selectedSeat = ref('')
const adminRows = ref([])
const statusText = ref('')
const wsConnected = ref(false)
let ws = null
let timer = null

const seatSummary = computed(() => {
  let available = 0
  let locked = 0
  let booked = 0
  for (const seat of seats.value) {
    if (seat.status === 'BOOKED') booked += 1
    else if (seat.status === 'LOCKED') locked += 1
    else available += 1
  }
  return { available, locked, booked }
})

async function login() {
  statusText.value = ''
  const res = await fetch(`${apiBase}/auth/mock`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ user_id: userId.value, role: role.value })
  })
  const data = await res.json()
  if (!res.ok) {
    statusText.value = data.error || 'login failed'
    return
  }
  token.value = data.token
  await loadSeats()
  connectWs()
  if (role.value === 'ADMIN') {
    await loadAdminBookings()
  }
}

async function loadSeats() {
  if (!token.value) return
  const res = await fetch(`${apiBase}/shows/${showId.value}/seats`, {
    headers: { Authorization: `Bearer ${token.value}` }
  })
  const data = await res.json()
  if (!res.ok) {
    statusText.value = data.error || 'cannot load seats'
    return
  }
  seats.value = data.seats
}

async function lockSeat(seatId) {
  const res = await fetch(`${apiBase}/shows/${showId.value}/seats/${seatId}/lock`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token.value}` }
  })
  const data = await res.json()
  statusText.value = res.ok ? `locked ${seatId}` : data.error
  if (res.ok) {
    selectedSeat.value = seatId
    await loadSeats()
  }
}

async function releaseSeat() {
  if (!selectedSeat.value) return
  const res = await fetch(`${apiBase}/shows/${showId.value}/seats/${selectedSeat.value}/lock`, {
    method: 'DELETE',
    headers: { Authorization: `Bearer ${token.value}` }
  })
  const data = await res.json()
  statusText.value = res.ok ? `released ${selectedSeat.value}` : data.error
  if (res.ok) {
    selectedSeat.value = ''
    await loadSeats()
  }
}

async function confirmBooking() {
  if (!selectedSeat.value) return
  const seatId = selectedSeat.value
  const res = await fetch(`${apiBase}/shows/${showId.value}/seats/${seatId}/confirm`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token.value}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({ movie: movie.value, show_date: showDate.value })
  })
  const data = await res.json()
  statusText.value = res.ok ? `booked ${seatId}` : data.error
  if (res.ok) {
    selectedSeat.value = ''
    await loadSeats()
    if (role.value === 'ADMIN') {
      await loadAdminBookings()
    }
  }
}

async function loadAdminBookings() {
  const q = new URLSearchParams({ movie: movie.value, date: showDate.value })
  const res = await fetch(`${apiBase}/admin/bookings?${q.toString()}`, {
    headers: { Authorization: `Bearer ${token.value}` }
  })
  const data = await res.json()
  if (!res.ok) {
    statusText.value = data.error || 'cannot load bookings'
    return
  }
  adminRows.value = data.items || []
}

function connectWs() {
  if (ws) ws.close()
  const wsUrl = apiBase.replace('http', 'ws') + `/ws/shows/${showId.value}`
  ws = new WebSocket(wsUrl)
  ws.onopen = () => {
    wsConnected.value = true
  }
  ws.onclose = () => {
    wsConnected.value = false
  }
  ws.onerror = () => {
    wsConnected.value = false
  }
  ws.onmessage = async () => {
    await loadSeats()
    if (role.value === 'ADMIN') {
      await loadAdminBookings()
    }
  }
}

function seatClass(seat) {
  return seat.status === 'BOOKED' ? 'booked' : seat.status === 'LOCKED' ? 'locked' : 'available'
}

function canClickSeat(seat) {
  if (seat.status === 'BOOKED') return false
  if (seat.status === 'LOCKED' && seat.locked_by !== userId.value) return false
  return true
}

onMounted(() => {
  timer = setInterval(() => {
    if (token.value) loadSeats()
  }, 3000)
})

onUnmounted(() => {
  if (ws) ws.close()
  if (timer) clearInterval(timer)
})
</script>

<template>
  <div class="page">
    <div class="hero">
      <div>
        <h1>Cinema Ticket Booking</h1>
      </div>
      <div class="chips">
        <span class="chip">API: {{ apiBase }}</span>
        <span class="chip" :class="wsConnected ? 'chip-ok' : 'chip-warn'">WS: {{ wsConnected ? 'connected' : 'disconnected' }}</span>
      </div>
    </div>

    <div class="layout">
      <section class="panel">
        <h2>Session</h2>
        <div class="form-grid">
          <label>
            User ID
            <input v-model="userId" placeholder="user id" />
          </label>
          <label>
            Role
            <select v-model="role">
              <option value="USER">USER</option>
              <option value="ADMIN">ADMIN</option>
            </select>
          </label>
          <label>
            Show ID
            <input v-model="showId" placeholder="show id" />
          </label>
          <label>
            Movie
            <input v-model="movie" placeholder="movie" />
          </label>
          <label>
            Show Date
            <input v-model="showDate" type="date" />
          </label>
        </div>
        <div class="actions">
          <button class="btn-primary" @click="login">Login</button>
        </div>
        <p class="status">status: {{ statusText || '-' }}</p>
      </section>

      <section class="panel" v-if="token">
        <div class="panel-head">
          <h2>Seat Map</h2>
          <div class="actions inline">
            <button @click="releaseSeat" :disabled="!selectedSeat">Release</button>
            <button class="btn-primary" @click="confirmBooking" :disabled="!selectedSeat">Confirm</button>
          </div>
        </div>

        <div class="summary-grid">
          <div class="summary-card available">Available {{ seatSummary.available }}</div>
          <div class="summary-card locked">Locked {{ seatSummary.locked }}</div>
          <div class="summary-card booked">Booked {{ seatSummary.booked }}</div>
        </div>

        <div class="seat-grid">
          <button
            v-for="seat in seats"
            :key="seat.seat_id"
            class="seat"
            :class="seatClass(seat)"
            :disabled="!canClickSeat(seat)"
            @click="lockSeat(seat.seat_id)"
          >
            <strong>{{ seat.seat_id }}</strong>
            <small>{{ seat.status }}</small>
          </button>
        </div>
        <p class="status">selected: {{ selectedSeat || '-' }}</p>
      </section>

      <section class="panel" v-if="token && role === 'ADMIN'">
        <div class="panel-head">
          <h2>Admin Dashboard</h2>
          <button @click="loadAdminBookings">Refresh</button>
        </div>
        <div class="table-wrap">
          <table class="table">
            <thead>
              <tr>
                <th>User</th>
                <th>Movie</th>
                <th>Date</th>
                <th>Show</th>
                <th>Seat</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="row in adminRows" :key="`${row.show_id}-${row.seat_id}`">
                <td>{{ row.user_id }}</td>
                <td>{{ row.movie }}</td>
                <td>{{ row.show_date }}</td>
                <td>{{ row.show_id }}</td>
                <td>{{ row.seat_id }}</td>
                <td>{{ row.status }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>
  </div>
</template>
