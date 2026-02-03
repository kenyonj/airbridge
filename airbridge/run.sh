#!/command/with-contenv bashio

# Get config options
LOG_LEVEL=$(bashio::config 'log_level')
PORT=$(bashio::config 'port')

bashio::log.info "Starting Airbridge..."
bashio::log.info "Log level: ${LOG_LEVEL}"
bashio::log.info "Port: ${PORT}"

# Export environment variables
export AIRBRIDGE_PORT="${PORT}"
export AIRBRIDGE_DB="/data/airbridge.db"

# Run airbridge in web mode
exec /usr/local/bin/airbridge --web
