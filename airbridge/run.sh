#!/command/with-contenv bashio

# Get config options
LOG_LEVEL=$(bashio::config 'log_level')

bashio::log.info "Starting Airbridge..."
bashio::log.info "Log level: ${LOG_LEVEL}"

# Run airbridge in web mode
exec /usr/local/bin/airbridge --web --db /data/airbridge.db --port 8200
