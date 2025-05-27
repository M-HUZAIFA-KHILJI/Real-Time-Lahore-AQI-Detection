package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/sony/gobreaker"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

// WeatherData represents the relevant fields we'll extract from the OpenWeatherMap API response.
type WeatherData struct {
	Temperature float64 `json:"temperature"`
	Humidity    int     `json:"humidity"`
	WindSpeed   float64 `json:"wind_speed"`
	Conditions  string  `json:"conditions"` // e.g., "clear sky", "few clouds"
}

// AQIData represents the relevant fields for Air Quality Index.
type AQIData struct {
	AQI   int     `json:"aqi"` // OpenWeatherMap's AQI (1-5)
	CO    float64 `json:"co"`
	NO    float64 `json:"no"`
	NO2   float64 `json:"no2"`
	O3    float64 `json:"o3"`
	SO2   float64 `json:"so2"`
	PM2_5 float64 `json:"pm2_5"`
	PM10  float64 `json:"pm10"`
	NH3   float64 `json:"nh3"`
}

// OpenWeatherMapClient handles fetching weather data from the OpenWeatherMap API.
type OpenWeatherMapClient struct {
	APIKey string
	Client *http.Client
}

// NewOpenWeatherMapClient creates and returns a new OpenWeatherMapClient instance.
func NewOpenWeatherMapClient(apiKey string) *OpenWeatherMapClient {
	return &OpenWeatherMapClient{
		APIKey: apiKey,
		Client: &http.Client{
			Timeout: 10 * time.Second, // Set a reasonable HTTP request timeout
		},
	}
}

// FetchWeather makes an API call to OpenWeatherMap to get current weather data
// for the given latitude and longitude.
func (c *OpenWeatherMapClient) FetchWeather(ctx context.Context, lat, lon float64) (*WeatherData, error) {
	// Construct the URL for current weather data, using 'metric' units for Celsius.
	url := fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?lat=%f&lon=%f&appid=%s&units=metric", lat, lon, c.APIKey)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body) // Read body to get more error details
		return nil, fmt.Errorf("OpenWeatherMap API returned non-OK status: %d, response: %s", resp.StatusCode, string(body))
	}

	// Define an anonymous struct to unmarshal only the necessary parts
	// of the large OpenWeatherMap JSON response.
	var owmResponse struct {
		Main struct {
			Temp     float64 `json:"temp"`
			Humidity int     `json:"humidity"`
		} `json:"main"`
		Wind struct {
			Speed float64 `json:"speed"`
		} `json:"wind"`
		Weather []struct {
			Description string `json:"description"`
		} `json:"weather"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&owmResponse); err != nil {
		return nil, fmt.Errorf("failed to decode OpenWeatherMap API response: %w", err)
	}

	conditions := ""
	if len(owmResponse.Weather) > 0 {
		conditions = owmResponse.Weather[0].Description
	}

	// Return the parsed WeatherData
	return &WeatherData{
		Temperature: owmResponse.Main.Temp,
		Humidity:    owmResponse.Main.Humidity,
		WindSpeed:   owmResponse.Wind.Speed,
		Conditions:  conditions,
	}, nil
}

// FetchAQI makes an API call to OpenWeatherMap to get current air pollution data.
func (c *OpenWeatherMapClient) FetchAQI(ctx context.Context, lat, lon float64) (*AQIData, error) {
	url := fmt.Sprintf("https://api.openweathermap.org/data/2.5/air_pollution?lat=%f&lon=%f&appid=%s", lat, lon, c.APIKey)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create AQI HTTP request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute AQI HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenWeatherMap AQI API returned non-OK status: %d, response: %s", resp.StatusCode, string(body))
	}

	var owmAQIPopResponse struct {
		List []struct {
			Main struct {
				AQI int `json:"aqi"`
			} `json:"main"`
			Components struct {
				CO    float64 `json:"co"`
				NO    float64 `json:"no"`
				NO2   float64 `json:"no2"`
				O3    float64 `json:"o3"`
				SO2   float64 `json:"so2"`
				PM2_5 float64 `json:"pm2_5"`
				PM10  float64 `json:"pm10"`
				NH3   float64 `json:"nh3"`
			} `json:"components"`
		} `json:"list"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&owmAQIPopResponse); err != nil {
		return nil, fmt.Errorf("failed to decode OpenWeatherMap AQI API response: %w", err)
	}

	if len(owmAQIPopResponse.List) == 0 {
		return nil, errors.New("no AQI data found in response")
	}

	aqiData := owmAQIPopResponse.List[0]
	return &AQIData{
		AQI:   aqiData.Main.AQI,
		CO:    aqiData.Components.CO,
		NO:    aqiData.Components.NO,
		NO2:   aqiData.Components.NO2,
		O3:    aqiData.Components.O3,
		SO2:   aqiData.Components.SO2,
		PM2_5: aqiData.Components.PM2_5,
		PM10:  aqiData.Components.PM10,
		NH3:   aqiData.Components.NH3,
	}, nil
}

// IngestedData combines weather and AQI for storage, including city name.
// Note the `bson` tags for mapping Go struct fields to MongoDB document fields.
type IngestedData struct {
	City        string    `bson:"city"`
	Latitude    float64   `bson:"latitude"`
	Longitude   float64   `bson:"longitude"`
	Temperature float64   `bson:"temperature"`
	Humidity    int       `bson:"humidity"`
	WindSpeed   float64   `bson:"wind_speed"`
	Conditions  string    `bson:"conditions"`
	AQI         int       `bson:"aqi"`
	CO          float64   `bson:"co"`
	NO          float64   `bson:"no"`
	NO2         float64   `bson:"no2"`
	O3          float64   `bson:"o3"`
	SO2         float64   `bson:"so2"`
	PM2_5       float64   `bson:"pm2_5"`
	PM10        float64   `bson:"pm10"`
	NH3         float64   `bson:"nh3"`
	Timestamp   time.Time `bson:"timestamp"`
}

// CityInfo to hold coordinates and name for each city.
type CityInfo struct {
	Name string
	Lat  float64
	Lon  float64
}

// Config holds application-wide settings for API interactions.
type Config struct {
	MaxRetries             int
	BaseRetryDelay         time.Duration
	MaxJitter              time.Duration
	FetchInterval          time.Duration // How often to fetch data
	APITimeout             time.Duration // Timeout for individual API calls
	CircuitBreakerSettings gobreaker.Settings
}

var appConfig = Config{
	MaxRetries:     3,
	BaseRetryDelay: 1 * time.Second,
	MaxJitter:      500 * time.Millisecond,
	FetchInterval:  9 * time.Minute, // Adjusted for 6 cities (Weather + AQI) to stay within 1000 AQI calls/day
	APITimeout:     20 * time.Second,
	CircuitBreakerSettings: gobreaker.Settings{
		Name:        "OpenWeatherMapCircuitBreaker",
		MaxRequests: 5,
		Interval:    30 * time.Second,
		Timeout:     1 * time.Minute,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 3
		},
	},
}

func main() {
	// --- HARDCODED CREDENTIALS (FOR IMMEDIATE TESTING ONLY) ---
	// IMPORTANT: For production, move these to environment variables or a secure config system.
	const openWeatherMapAPIKey = "f969cf3966509fa4294690528aaf419a"
	const mongoURI = "mongodb+srv://MHK_Technologies:nISQuhdTNSo1N7Lq@cluster0.lgzcnm2.mongodb.net/?retryWrites=true&w=majority"
	const mongoDatabaseName = "weather_aqi_db" // Changed database name to distinguish
	const mongoCollectionName = "city_data"    // Single collection for all cities
	// --- END HARDCODED CREDENTIALS ---

	// --- Define Cities to Monitor ---
	cities := []CityInfo{
		{Name: "Lahore", Lat: 31.5204, Lon: 74.3587},
		{Name: "Sheikhupura", Lat: 31.7167, Lon: 74.0000},
		{Name: "Kasur", Lat: 31.1167, Lon: 74.4500},
		{Name: "Amritsar", Lat: 31.6333, Lon: 74.8333}, // India
		{Name: "Gujranwala", Lat: 32.1667, Lon: 74.1833},
		{Name: "Sialkot", Lat: 32.4833, Lon: 74.5333},
	}

	// --- MongoDB Atlas Initialization ---
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("Error connecting to MongoDB Atlas: %v", err)
	}
	defer func() {
		if disconnectErr := client.Disconnect(context.Background()); disconnectErr != nil {
			log.Printf("Error disconnecting from MongoDB: %v", disconnectErr)
		}
	}()

	// Ping the primary to ensure connection is established
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err = client.Ping(ctx, readpref.Primary()); err != nil {
		log.Fatalf("Could not ping MongoDB Atlas. Please check your MONGO_URI, network access, and user credentials: %v", err)
	}
	fmt.Println("Successfully connected to MongoDB Atlas!")

	// Get a handle to the desired database and collection
	collection := client.Database(mongoDatabaseName).Collection(mongoCollectionName)

	// --- OpenWeatherMap Client Initialization ---
	owmClient := NewOpenWeatherMapClient(openWeatherMapAPIKey)

	// --- Circuit Breaker Setup for OpenWeatherMap ---
	owmCircuitBreaker := gobreaker.NewCircuitBreaker(appConfig.CircuitBreakerSettings)

	// --- Channel for Data Storage ---
	dataToStoreChan := make(chan IngestedData, 100) // Larger buffer for multiple cities

	// --- Goroutine for Database Insertion ---
	go func() {
		for data := range dataToStoreChan {
			if err := insertWeatherData(collection, data); err != nil { // Renamed func for clarity
				log.Printf("ERROR: Failed to insert data for %s into MongoDB: %v", data.City, err)
			} else {
				log.Printf("INFO: Successfully inserted data for %s at %s. Data: %+v into MongoDB", data.City, data.Timestamp.Format(time.RFC3339), data)
			}
		}
	}()

	// --- Periodic Data Fetching Loop ---
	log.Printf("Starting periodic data ingestion for %d cities (every %s) into MongoDB...", len(cities), appConfig.FetchInterval)
	ticker := time.NewTicker(appConfig.FetchInterval)
	defer ticker.Stop() // Ensure the ticker is stopped when main exits

	for range ticker.C {
		// Iterate through each city
		for _, city := range cities {
			currentCity := city // Create a local copy for goroutine closure
			// We'll run each city's fetch concurrently to speed up the loop,
			// but still respect the overall interval.
			go func() {
				log.Printf("INFO: Attempting to fetch weather and AQI data for %s...", currentCity.Name)

				// Execute the fetch operation via the circuit breaker.
				_, err := owmCircuitBreaker.Execute(func() (interface{}, error) {
					for attempt := 0; attempt < appConfig.MaxRetries; attempt++ {
						if attempt > 0 {
							delay := appConfig.BaseRetryDelay + time.Duration(rand.Float64()*float64(appConfig.MaxJitter))
							time.Sleep(delay)
							log.Printf("INFO: Retrying OpenWeatherMap fetch for %s (attempt %d/%d)...", currentCity.Name, attempt+1, appConfig.MaxRetries)
						}

						apiCtx, apiCancel := context.WithTimeout(context.Background(), appConfig.APITimeout)
						weatherData, fetchWeatherErr := owmClient.FetchWeather(apiCtx, currentCity.Lat, currentCity.Lon)
						aqiData, fetchAQIErr := owmClient.FetchAQI(apiCtx, currentCity.Lat, currentCity.Lon)
						apiCancel()

						// If either API call fails, retry both for this city
						if fetchWeatherErr != nil || fetchAQIErr != nil {
							return nil, fmt.Errorf("failed to fetch data for %s: Weather error: %v, AQI error: %v", currentCity.Name, fetchWeatherErr, fetchAQIErr)
						}

						// Validate data before sending to channel
						if validateWeatherData(weatherData) && validateAQIData(aqiData) {
							// Data is valid, prepare for storage and send to channel.
							dataToStoreChan <- IngestedData{
								City:        currentCity.Name,
								Latitude:    currentCity.Lat,
								Longitude:   currentCity.Lon,
								Temperature: weatherData.Temperature,
								Humidity:    weatherData.Humidity,
								WindSpeed:   weatherData.WindSpeed,
								Conditions:  weatherData.Conditions,
								AQI:         aqiData.AQI,
								CO:          aqiData.CO,
								NO:          aqiData.NO,
								NO2:         aqiData.NO2,
								O3:          aqiData.O3,
								SO2:         aqiData.SO2,
								PM2_5:       aqiData.PM2_5,
								PM10:        aqiData.PM10,
								NH3:         aqiData.NH3,
								Timestamp:   time.Now(), // Record the time of ingestion
							}
							return nil, nil // Indicate success to the circuit breaker
						} else {
							return nil, fmt.Errorf("received invalid data from OpenWeatherMap for %s: Weather: %+v, AQI: %+v", currentCity.Name, weatherData, aqiData)
						}
					}
					return nil, fmt.Errorf("all %d retries failed for OpenWeatherMap API for %s", appConfig.MaxRetries, currentCity.Name)
				})

				// Handle errors from the circuit breaker execution for this city.
				if err != nil {
					if errors.Is(err, gobreaker.ErrOpenState) {
						log.Printf("WARNING: Circuit breaker is OPEN for OpenWeatherMap API. Skipping current fetch for %s.", currentCity.Name)
					} else {
						log.Printf("ERROR: OpenWeatherMap data fetch for %s failed after retries or due to circuit breaker: %v", currentCity.Name, err)
					}
				}
			}() // End of goroutine for currentCity
		} // End of cities loop
	} // End of ticker loop
}

// insertWeatherData inserts a parsed IngestedData struct into the MongoDB collection.
func insertWeatherData(collection *mongo.Collection, data IngestedData) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.InsertOne(ctx, data)
	return err
}

// validateWeatherData performs basic validation on the fetched weather data.
func validateWeatherData(data *WeatherData) bool {
	// Simple range checks for plausibility
	return data != nil &&
		data.Temperature > -50 && data.Temperature < 70 && // Reasonable temperature range in Celsius
		data.Humidity >= 0 && data.Humidity <= 100 && // Humidity between 0-100%
		data.WindSpeed >= 0 // Wind speed non-negative
}

// validateAQIData performs basic validation on the fetched AQI data.
func validateAQIData(data *AQIData) bool {
	// OpenWeatherMap AQI is 1-5
	return data != nil && data.AQI >= 1 && data.AQI <= 5
}

// Initialize random source for jitter.
// var r = rand.New(rand.NewSource(time.Now().UnixNano()))
