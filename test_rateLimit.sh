#!/bin/bash

# Define the URL and request data
URL="http://127.0.0.1:11111/"
DATA='{
    "method": "getblockhash",
    "params": [
        "LTC",
        2503481
    ]
}'

# Set the number of requests you want to send
NUM_REQUESTS=100

# Loop to send requests and measure response times
for ((i=1; i<=$NUM_REQUESTS; i++)); do
  # Measure the time it takes to send a request and get a response
  RESPONSE_TIME=$( (time -p curl --location "$URL" --header 'Content-Type: application/json' --data "$DATA") 2>&1 | grep real | cut -f2 -d' ' )
  echo "Request $i of $NUM_REQUESTS took $RESPONSE_TIME seconds"
done
