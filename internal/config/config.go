package config

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var PORT string
var AllowedOrigins string
var DB *mongo.Database
var BotToken string
var OllamaDefaultModel string
var OllamaBaseUrl string
var OwnerOnly string

func envPath() string {
	_, b, _, _ := runtime.Caller(0)
	basePath := filepath.Join(filepath.Dir(b), "../..")
	envPath := filepath.Join(basePath, ".env")
	return envPath
}

func LoadConfig() {
	path := envPath()
	err := godotenv.Load(path)
	log.Println("Load .env file", path)
	if err != nil {
		log.Println("Error loading .env file, using environment variables")
	}

	PORT = ":" + os.Getenv("PORT")
	AllowedOrigins = os.Getenv("ALLOWED_ORIGINS")
	mongoURI := os.Getenv("MONGODB_URI")
	dbName := os.Getenv("DB_NAME")
	BotToken = os.Getenv("BOT_TOKEN")
	OllamaDefaultModel = os.Getenv("OLLAMA_DEFAULT_MODEL")
	OllamaBaseUrl = os.Getenv("OLLAMA_BASE_URL")
	OwnerOnly = os.Getenv("OWNER_ONLY")

	if AllowedOrigins == "" {
		AllowedOrigins = "*"
	}

	ConnectMongoDB(mongoURI, dbName)
}

func ConnectMongoDB(mongoURI, dbName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	clientOptions := options.Client().ApplyURI(mongoURI)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Fatal(err)
	}

	err = client.Ping(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Connected to MongoDB!")
	DB = client.Database(dbName)
}
