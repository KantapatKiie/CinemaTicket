package main

import (
	"encoding/base64"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"cinema-ticket/backend/internal/seatmap"
	"google.golang.org/api/idtoken"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type App struct {
	cfg           Config
	redis          *redis.Client
	mongo          *mongo.Client
	bookings       *mongo.Collection
	auditLogs      *mongo.Collection
	notifications  *mongo.Collection
	hub            *Hub
	jwtSecretBytes []byte
}

type Config struct {
	Port           string
	MongoURI       string
	MongoDB        string
	RedisAddr      string
	RedisPassword  string
	RedisDB        int
	JWTSecret      string
	FirebaseProjectID string
	LockTTLSeconds int
	SeatRows       int
	SeatCols       int
}

type UserClaims struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

type Booking struct {
	ID        interface{} `bson:"_id,omitempty" json:"id"`
	ShowID    string      `bson:"show_id" json:"show_id"`
	SeatID    string      `bson:"seat_id" json:"seat_id"`
	UserID    string      `bson:"user_id" json:"user_id"`
	Movie     string      `bson:"movie" json:"movie"`
	ShowDate  string      `bson:"show_date" json:"show_date"`
	Status    string      `bson:"status" json:"status"`
	CreatedAt time.Time   `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time   `bson:"updated_at" json:"updated_at"`
}

type AuditLog struct {
	ID        interface{} `bson:"_id,omitempty" json:"id"`
	EventType string      `bson:"event_type" json:"event_type"`
	ShowID    string      `bson:"show_id" json:"show_id"`
	SeatID    string      `bson:"seat_id" json:"seat_id"`
	UserID    string      `bson:"user_id" json:"user_id"`
	Payload   interface{} `bson:"payload" json:"payload"`
	CreatedAt time.Time   `bson:"created_at" json:"created_at"`
}

type Seat struct {
	SeatID    string `json:"seat_id"`
	Status    string `json:"status"`
	LockedBy  string `json:"locked_by,omitempty"`
	ExpiresIn int64  `json:"expires_in,omitempty"`
}

type SeatEvent struct {
	Type      string    `json:"type"`
	ShowID    string    `json:"show_id"`
	SeatID    string    `json:"seat_id"`
	Status    string    `json:"status"`
	UserID    string    `json:"user_id"`
	Timestamp time.Time `json:"timestamp"`
}

type Hub struct {
	mu    sync.RWMutex
	rooms map[string]map[*websocket.Conn]bool
}

func NewHub() *Hub {
	return &Hub{rooms: map[string]map[*websocket.Conn]bool{}}
}

func (h *Hub) Add(showID string, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.rooms[showID]; !ok {
		h.rooms[showID] = map[*websocket.Conn]bool{}
	}
	h.rooms[showID][c] = true
}

func (h *Hub) Remove(showID string, c *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.rooms[showID]; !ok {
		return
	}
	delete(h.rooms[showID], c)
	if len(h.rooms[showID]) == 0 {
		delete(h.rooms, showID)
	}
}

func (h *Hub) Broadcast(showID string, message []byte) {
	h.mu.RLock()
	connections := h.rooms[showID]
	h.mu.RUnlock()
	for conn := range connections {
		_ = conn.WriteMessage(websocket.TextMessage, message)
	}
}

func envOrDefault(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func intEnvOrDefault(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return i
}

func loadConfig() Config {
	return Config{
		Port:           envOrDefault("PORT", "8080"),
		MongoURI:       envOrDefault("MONGO_URI", "mongodb://mongo:27017"),
		MongoDB:        envOrDefault("MONGO_DB", "cinema"),
		RedisAddr:      envOrDefault("REDIS_ADDR", "redis:6379"),
		RedisPassword:  envOrDefault("REDIS_PASSWORD", ""),
		RedisDB:        intEnvOrDefault("REDIS_DB", 0),
		JWTSecret:      envOrDefault("JWT_SECRET", "dev-secret"),
		FirebaseProjectID: envOrDefault("FIREBASE_PROJECT_ID", ""),
		LockTTLSeconds: intEnvOrDefault("LOCK_TTL_SECONDS", 300),
		SeatRows:       intEnvOrDefault("SEAT_ROWS", 5),
		SeatCols:       intEnvOrDefault("SEAT_COLS", 10),
	}
}

func main() {
	cfg := loadConfig()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		log.Fatal(err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB})
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatal(err)
	}

	db := mongoClient.Database(cfg.MongoDB)
	app := &App{
		cfg:           cfg,
		redis:         rdb,
		mongo:         mongoClient,
		bookings:      db.Collection("bookings"),
		auditLogs:     db.Collection("audit_logs"),
		notifications: db.Collection("notifications"),
		hub:           NewHub(),
		jwtSecretBytes: []byte(cfg.JWTSecret),
	}

	app.ensureIndexes(context.Background())
	go app.consumeBookingEvents()
	go app.consumeSeatEvents()

	r := gin.Default()
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.POST("/auth/mock", app.mockLogin)

	auth := r.Group("/")
	auth.Use(app.authMiddleware())
	auth.GET("/shows/:showID/seats", app.getSeatMap)
	auth.POST("/shows/:showID/seats/:seatID/lock", app.lockSeat)
	auth.DELETE("/shows/:showID/seats/:seatID/lock", app.releaseSeat)
	auth.POST("/shows/:showID/seats/:seatID/confirm", app.confirmSeat)
	auth.GET("/ws/shows/:showID", app.wsSeatUpdates)

	admin := auth.Group("/admin")
	admin.Use(app.requireRole("ADMIN"))
	admin.GET("/bookings", app.listBookings)
	admin.GET("/audit-logs", app.listAuditLogs)

	log.Fatal(r.Run(":" + cfg.Port))
}

func (a *App) ensureIndexes(ctx context.Context) {
	_, _ = a.bookings.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "show_id", Value: 1}, {Key: "seat_id", Value: 1}, {Key: "status", Value: 1}}, Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"status": "BOOKED"})},
		{Keys: bson.D{{Key: "movie", Value: 1}}},
		{Keys: bson.D{{Key: "show_date", Value: 1}}},
		{Keys: bson.D{{Key: "user_id", Value: 1}}},
	})
}

func (a *App) mockLogin(c *gin.Context) {
	var body struct {
		UserID string `json:"user_id"`
		Role   string `json:"role"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if body.UserID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
		return
	}
	role := strings.ToUpper(body.Role)
	if role == "" {
		role = "USER"
	}
	if role != "USER" && role != "ADMIN" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be USER or ADMIN"})
		return
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, UserClaims{
		UserID: body.UserID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	s, err := token.SignedString(a.jwtSecretBytes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot sign token"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": s, "user_id": body.UserID, "role": role})
}

func (a *App) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if h == "" || !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		raw := strings.TrimPrefix(h, "Bearer ")
		if a.cfg.FirebaseProjectID != "" && isFirebaseToken(raw) {
			t, err := idtoken.Validate(c.Request.Context(), raw, a.cfg.FirebaseProjectID)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid firebase token"})
				return
			}
			userID, _ := t.Claims["user_id"].(string)
			if userID == "" {
				userID, _ = t.Claims["sub"].(string)
			}
			if userID == "" {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing firebase user"})
				return
			}
			role, _ := t.Claims["role"].(string)
			role = strings.ToUpper(role)
			if role == "" {
				role = "USER"
			}
			c.Set("user_id", userID)
			c.Set("role", role)
			c.Next()
			return
		}
		token, err := jwt.ParseWithClaims(raw, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
			return a.jwtSecretBytes, nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		claims, ok := token.Claims.(*UserClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid claims"})
			return
		}
		c.Set("user_id", claims.UserID)
		c.Set("role", claims.Role)
		c.Next()
	}
}

func (a *App) requireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		r := c.GetString("role")
		if r != role {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}

func (a *App) getSeatMap(c *gin.Context) {
	showID := c.Param("showID")
	baseSeats := seatmap.Build(a.cfg.SeatRows, a.cfg.SeatCols)
	seats := make([]Seat, 0, len(baseSeats))
	for _, s := range baseSeats {
		seats = append(seats, Seat{SeatID: s.SeatID, Status: s.Status})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cur, err := a.bookings.Find(ctx, bson.M{"show_id": showID, "status": "BOOKED"})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot read bookings"})
		return
	}
	defer cur.Close(ctx)

	booked := map[string]bool{}
	for cur.Next(ctx) {
		var b Booking
		_ = cur.Decode(&b)
		booked[b.SeatID] = true
	}

	keys := make([]string, 0, len(seats))
	for _, s := range seats {
		keys = append(keys, seatmap.LockKey(showID, s.SeatID))
	}
	values, _ := a.redis.MGet(ctx, keys...).Result()

	for i := range seats {
		if booked[seats[i].SeatID] {
			seats[i].Status = "BOOKED"
			continue
		}
		if values[i] != nil {
			seats[i].Status = "LOCKED"
			seats[i].LockedBy = fmt.Sprintf("%v", values[i])
			ttl := a.redis.TTL(ctx, keys[i]).Val()
			if ttl > 0 {
				seats[i].ExpiresIn = int64(ttl.Seconds())
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"show_id": showID, "seats": seats})
}

func (a *App) lockSeat(c *gin.Context) {
	showID := c.Param("showID")
	seatID := strings.ToUpper(c.Param("seatID"))
	userID := c.GetString("user_id")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, _ := a.bookings.CountDocuments(ctx, bson.M{"show_id": showID, "seat_id": seatID, "status": "BOOKED"})
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "seat already booked"})
		return
	}

	ok, err := a.redis.SetNX(ctx, seatmap.LockKey(showID, seatID), userID, time.Duration(a.cfg.LockTTLSeconds)*time.Second).Result()
	if err != nil {
		a.writeAudit("SYSTEM_ERROR", showID, seatID, userID, gin.H{"error": err.Error()})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot lock seat"})
		return
	}
	if !ok {
		owner, _ := a.redis.Get(ctx, seatmap.LockKey(showID, seatID)).Result()
		c.JSON(http.StatusConflict, gin.H{"error": "seat already locked", "locked_by": owner})
		return
	}

	a.publishSeatEvent(SeatEvent{Type: "SEAT_LOCKED", ShowID: showID, SeatID: seatID, Status: "LOCKED", UserID: userID, Timestamp: time.Now()})
	a.writeAudit("SEAT_LOCKED", showID, seatID, userID, nil)
	c.JSON(http.StatusOK, gin.H{"status": "LOCKED", "show_id": showID, "seat_id": seatID, "ttl": a.cfg.LockTTLSeconds})
}

func (a *App) releaseSeat(c *gin.Context) {
	showID := c.Param("showID")
	seatID := strings.ToUpper(c.Param("seatID"))
	userID := c.GetString("user_id")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := seatmap.LockKey(showID, seatID)
	owner, err := a.redis.Get(ctx, key).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot read lock"})
		return
	}
	if owner == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "lock not found"})
		return
	}
	if owner != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot release another user lock"})
		return
	}
	_, _ = a.redis.Del(ctx, key).Result()
	a.publishSeatEvent(SeatEvent{Type: "SEAT_RELEASED", ShowID: showID, SeatID: seatID, Status: "AVAILABLE", UserID: userID, Timestamp: time.Now()})
	a.writeAudit("SEAT_RELEASED", showID, seatID, userID, nil)
	c.JSON(http.StatusOK, gin.H{"status": "AVAILABLE", "show_id": showID, "seat_id": seatID})
}

func (a *App) confirmSeat(c *gin.Context) {
	showID := c.Param("showID")
	seatID := strings.ToUpper(c.Param("seatID"))
	userID := c.GetString("user_id")
	var body struct {
		Movie    string `json:"movie"`
		ShowDate string `json:"show_date"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := seatmap.LockKey(showID, seatID)
	owner, err := a.redis.Get(ctx, key).Result()
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "lock expired"})
		return
	}
	if owner != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "not lock owner"})
		return
	}

	now := time.Now()
	_, err = a.bookings.UpdateOne(ctx,
		bson.M{"show_id": showID, "seat_id": seatID},
		bson.M{"$set": bson.M{"show_id": showID, "seat_id": seatID, "user_id": userID, "movie": body.Movie, "show_date": body.ShowDate, "status": "BOOKED", "updated_at": now}, "$setOnInsert": bson.M{"created_at": now}},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "seat already booked"})
		return
	}

	_, _ = a.redis.Del(ctx, key).Result()
	payload := gin.H{"show_id": showID, "seat_id": seatID, "user_id": userID, "movie": body.Movie, "show_date": body.ShowDate}
	a.publishSeatEvent(SeatEvent{Type: "BOOKING_SUCCESS", ShowID: showID, SeatID: seatID, Status: "BOOKED", UserID: userID, Timestamp: now})
	a.publishBookingEvent(payload)
	a.writeAudit("BOOKING_SUCCESS", showID, seatID, userID, payload)
	c.JSON(http.StatusOK, gin.H{"status": "BOOKED", "show_id": showID, "seat_id": seatID, "user_id": userID})
}

func (a *App) listBookings(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{}
	if movie := c.Query("movie"); movie != "" {
		filter["movie"] = movie
	}
	if date := c.Query("date"); date != "" {
		filter["show_date"] = date
	}
	if user := c.Query("user"); user != "" {
		filter["user_id"] = user
	}

	cur, err := a.bookings.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "updated_at", Value: -1}}))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot query bookings"})
		return
	}
	defer cur.Close(ctx)
	items := []Booking{}
	for cur.Next(ctx) {
		var b Booking
		_ = cur.Decode(&b)
		items = append(items, b)
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (a *App) listAuditLogs(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	filter := bson.M{}
	if eventType := c.Query("event"); eventType != "" {
		filter["event_type"] = eventType
	}
	if user := c.Query("user"); user != "" {
		filter["user_id"] = user
	}
	if showID := c.Query("show_id"); showID != "" {
		filter["show_id"] = showID
	}
	if seatID := c.Query("seat_id"); seatID != "" {
		filter["seat_id"] = strings.ToUpper(seatID)
	}

	cur, err := a.auditLogs.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "cannot query audit logs"})
		return
	}
	defer cur.Close(ctx)
	items := []AuditLog{}
	for cur.Next(ctx) {
		var l AuditLog
		_ = cur.Decode(&l)
		items = append(items, l)
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (a *App) wsSeatUpdates(c *gin.Context) {
	showID := c.Param("showID")
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	a.hub.Add(showID, conn)
	defer func() {
		a.hub.Remove(showID, conn)
		_ = conn.Close()
	}()
	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

func (a *App) publishSeatEvent(event SeatEvent) {
	b, _ := json.Marshal(event)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = a.redis.Publish(ctx, "show_events", b).Err()
	a.hub.Broadcast(event.ShowID, b)
}

func (a *App) consumeSeatEvents() {
	ctx := context.Background()
	sub := a.redis.Subscribe(ctx, "show_events")
	ch := sub.Channel()
	for msg := range ch {
		var event SeatEvent
		if err := json.Unmarshal([]byte(msg.Payload), &event); err == nil {
			a.hub.Broadcast(event.ShowID, []byte(msg.Payload))
		}
	}
}

func (a *App) publishBookingEvent(payload interface{}) {
	b, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = a.redis.Publish(ctx, "booking_events", b).Err()
}

func (a *App) consumeBookingEvents() {
	ctx := context.Background()
	sub := a.redis.Subscribe(ctx, "booking_events")
	ch := sub.Channel()
	for msg := range ch {
		var payload interface{}
		_ = json.Unmarshal([]byte(msg.Payload), &payload)
		_, _ = a.notifications.InsertOne(ctx, bson.M{"event_type": "BOOKING_SUCCESS", "payload": payload, "created_at": time.Now()})
	}
}

func (a *App) writeAudit(eventType, showID, seatID, userID string, payload interface{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, _ = a.auditLogs.InsertOne(ctx, AuditLog{EventType: eventType, ShowID: showID, SeatID: seatID, UserID: userID, Payload: payload, CreatedAt: time.Now()})
}

func isFirebaseToken(raw string) bool {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims map[string]interface{}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return false
	}
	iss, _ := claims["iss"].(string)
	return strings.HasPrefix(iss, "https://securetoken.google.com/")
}
