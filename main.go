package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/sony/sonyflake"
)

var ApplicationDescription string = "TaskQ Redis Publisher"
var BuildVersion string = "0.0.0"

var Debug bool = false
var DebugMetricsNotifierPeriod time.Duration = 60
var ListLengthWatcherPeriod time.Duration = 60

type RedisRPushRequestStruct struct {
	Channel string          `json:"channel"`
	Payload json.RawMessage `json:"payload"`
}

type ConfigurationStruct struct {
	RedisAddress  *net.TCPAddr `json:"redis_address"`
	ListenAddress *net.TCPAddr `json:"listen_address"`
}

type handleSignalParamsStruct struct {
	httpServer http.Server
}

type MetricsStruct struct {
	Index           int32
	Warnings        int32
	Errors          int32
	Success         int32
	Put             int32
	Started         time.Time
	WatchedChannels map[string]int64
}

var Configuration = ConfigurationStruct{}
var handleSignalParams = handleSignalParamsStruct{}

var watchedRedisChannels = make(map[string]time.Time)

var MetricsNotifierPeriod int = 60
var Metrics = MetricsStruct{
	Index:           0,
	Warnings:        0,
	Errors:          0,
	Success:         0,
	Put:             0,
	Started:         time.Now(),
	WatchedChannels: make(map[string]int64),
}

var ctx = context.Background()
var flake = sonyflake.NewSonyflake(sonyflake.Settings{})

var rdb *redis.Client

func MetricsNotifier() {
	go func() {
		for {
			time.Sleep(DebugMetricsNotifierPeriod * time.Second)
			log.Debug().
				Int32("Index", Metrics.Index).
				Int32("Put", Metrics.Put).
				Int32("Warnings", Metrics.Warnings).
				Int32("Errors", Metrics.Errors).
				Int32("Success", Metrics.Success).
				Msg("Metrics")
		}
	}()
}

func ListLengthWatcher() {
	go func() {
		for {

			for channel := range watchedRedisChannels {

				log.Debug().
					Str("channel", channel).
					Time("last_seen", watchedRedisChannels[channel]).
					Msgf("Checking channel length")

				result := rdb.LLen(ctx, channel)

				log.Debug().
					Str("channel", channel).
					Time("last_seen", watchedRedisChannels[channel]).
					Int64("length", result.Val()).
					Msgf("LLEN results for watched channel")

				Metrics.WatchedChannels[channel] = result.Val()

			}

			time.Sleep(ListLengthWatcherPeriod * time.Second)
		}
	}()
}

func handleSignal() {

	log.Debug().Msg("Initialising signal handling function")

	signalChannel := make(chan os.Signal)
	signal.Notify(signalChannel, os.Interrupt, syscall.SIGTERM)

	go func() {

		<-signalChannel

		err := handleSignalParams.httpServer.Shutdown(context.Background())

		if err != nil {
			log.Error().Err(err).Msgf("HTTP server Shutdown: %v", err)

		} else {
			log.Info().Msgf("HTTP server Shutdown complete")
		}

		log.Warn().Msg("SIGINT")
		os.Exit(0)

	}()
}

func handlerIndex(rw http.ResponseWriter, req *http.Request) {
	_ = atomic.AddInt32(&Metrics.Index, 1)
	fmt.Fprintf(rw, "%s v%s\n", html.EscapeString(ApplicationDescription), html.EscapeString(BuildVersion))
}

func handlerMetrics(rw http.ResponseWriter, req *http.Request) {

	fmt.Fprintf(rw, "# TYPE taskq_publisher_channel_len counter\n")
	fmt.Fprintf(rw, "# HELP Last checked Redis channel length\n")

	for channel := range Metrics.WatchedChannels {
		fmt.Fprintf(rw, "taskq_publisher_channel_len{channel=\"%v\"} %v\n", channel, Metrics.WatchedChannels[channel])
	}

	fmt.Fprintf(rw, "# TYPE taskq_publisher_requests counter\n")
	fmt.Fprintf(rw, "# HELP Number of the requests to the TaskQ Publisher by type\n")
	fmt.Fprintf(rw, "taskq_publisher_requests{method=\"put\"} %v\n", Metrics.Put)

	fmt.Fprintf(rw, "# TYPE taskq_publisher_errors counter\n")
	fmt.Fprintf(rw, "# HELP Number of the raised errors\n")
	fmt.Fprintf(rw, "taskq_publisher_errors %v\n", Metrics.Errors)

	fmt.Fprintf(rw, "# TYPE taskq_publisher_index counter\n")
	fmt.Fprintf(rw, "# HELP Number of the requests to /\n")
	fmt.Fprintf(rw, "taskq_publisher_index %v\n", Metrics.Index)

}

func handlerPut(rw http.ResponseWriter, req *http.Request) {

	log.Info().Msgf("Processing incoming request %v", req.URL)
	_ = atomic.AddInt32(&Metrics.Put, 1)

	RedisRPushRequest := RedisRPushRequestStruct{}

	log.Debug().Msgf("Processing request data")
	JSONDecoder := json.NewDecoder(req.Body)

	err := JSONDecoder.Decode(&RedisRPushRequest)
	if err != nil {
		_ = atomic.AddInt32(&Metrics.Errors, 1)
		log.Err(err).Msgf("Error while JSON decoding the API request")
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	uid, err := flake.NextID()
	if err != nil {
		_ = atomic.AddInt32(&Metrics.Errors, 1)
		log.Error().Err(err).Msgf("flake.NextID() failed")
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Debug().
		Uint64("uid", uid).
		Str("channel", RedisRPushRequest.Channel).
		Str("payload", string(RedisRPushRequest.Payload)).
		Int("payload_size", len(RedisRPushRequest.Payload)).
		Msgf("Publishing message")

	log.Info().
		Uint64("uid", uid).
		Str("channel", RedisRPushRequest.Channel).
		Int("payload_size", len(RedisRPushRequest.Payload)).
		Msgf("Publishing message")

	err = rdb.RPush(ctx, RedisRPushRequest.Channel, string(RedisRPushRequest.Payload)).Err()
	if err != nil {
		_ = atomic.AddInt32(&Metrics.Errors, 1)
		log.Error().Err(err).Msgf("Couldn't publish message")
		rw.WriteHeader(http.StatusInternalServerError)
		return
	}

	log.Info().
		Uint64("uid", uid).
		Str("channel", RedisRPushRequest.Channel).
		Int("payload_size", len(RedisRPushRequest.Payload)).
		Msgf("Published message successfully")

	// now := time.Now()

	// watchedRedisChannels[RedisRPushRequest.Channel] = now

	// log.Debug().
	// 	Str("channel", RedisRPushRequest.Channel).
	// 	Msgf("Channel added to LLEN wathcher")

	rw.WriteHeader(http.StatusOK)
	return

}

func init() {

	bindPtr := flag.String("bind", "127.0.0.1:8080", "Address and port to listen")
	redisAddressPtr := flag.String("redis-address", "127.0.0.1:6379", "Address and port of the Redis server")
	verbosePtr := flag.Bool("verbose", false, "Verbose output")
	showVersionPtr := flag.Bool("version", false, "Show version")

	flag.Parse()

	if *showVersionPtr {
		fmt.Printf("%s\n", ApplicationDescription)
		fmt.Printf("Version: %s\n", BuildVersion)
		os.Exit(0)
	}

	if *verbosePtr {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		MetricsNotifier()
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	log.Debug().Msg("Logger initialised")

	redis_address, err := net.ResolveTCPAddr("tcp4", *redisAddressPtr)
	if err != nil {
		log.Fatal().Err(err).Msgf("Error while resolving Redis server address")
	}

	Configuration.RedisAddress = redis_address

	listen_address, err := net.ResolveTCPAddr("tcp4", *bindPtr)
	if err != nil {
		log.Fatal().Err(err).Msgf("Error while resolving bind address")
	}

	Configuration.ListenAddress = listen_address

	handleSignal()
	// ListLengthWatcher()

}

func main() {

	log.Info().Msgf("Preparing Redis connection")
	log.Info().Msgf("Redis server address %s", Configuration.RedisAddress.String())

	rdb = redis.NewClient(&redis.Options{
		Addr: Configuration.RedisAddress.String(),
		// TODO: Impl Redis password
		// Password: os.Getenv("REDIS_PASSWORD"),
		PoolSize: 2000,
	})

	log.Info().Msgf("Listening on %s", Configuration.ListenAddress.String())

	srv := &http.Server{
		Addr:         Configuration.ListenAddress.String(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	handleSignalParams.httpServer = *srv

	http.HandleFunc("/", handlerIndex)
	http.HandleFunc("/put", handlerPut)
	http.HandleFunc("/metrics", handlerMetrics)

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal().Err(err).Msgf("HTTP server ListenAndServe error")
	}

}
