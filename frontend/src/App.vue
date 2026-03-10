<script setup>
import { onMounted, onUnmounted, ref } from 'vue'

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
let ws = null
let timer = null

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
  <div class="container">
    <div class="card">
      <h2>Cinema Ticket Booking</h2>
      <div class="row">
        <input v-model="userId" placeholder="user id" />
        <select v-model="role">
          <option value="USER">USER</option>
          <option value="ADMIN">ADMIN</option>
        </select>
        <input v-model="showId" placeholder="show id" />
        <input v-model="movie" placeholder="movie" />
        <input v-model="showDate" type="date" />
        <button @click="login">Login</button>
      </div>
      <div class="small">status: {{ statusText || '-' }}</div>
    </div>

    <div class="card" v-if="token">
      <div class="row" style="justify-content: space-between;">
        <h3>Seat Map</h3>
        <div class="row">
          <button @click="releaseSeat" :disabled="!selectedSeat">Release</button>
          <button @click="confirmBooking" :disabled="!selectedSeat">Confirm Booking</button>
        </div>
      </div>
      <div class="legend">
        <span style="background: #d1fae5">AVAILABLE</span>
        <span style="background: #fde68a">LOCKED</span>
        <span style="background: #fecaca">BOOKED</span>
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
          {{ seat.seat_id }}
        </button>
      </div>
      <div class="small" style="margin-top: 10px;">selected: {{ selectedSeat || '-' }}</div>
    </div>

    <div class="card" v-if="token && role === 'ADMIN'">
      <div class="row" style="justify-content: space-between;">
        <h3>Admin Dashboard</h3>
        <button @click="loadAdminBookings">Refresh</button>
      </div>
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
  </div>
</template>
