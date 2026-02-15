// Crypto Market Report — fetch prices, analyze, send to Slack
crypto "BTC,ETH,SOL,AVAX,LINK" -> ask "Create a brief crypto market report with price action highlights and which coin looks strongest" -> notify "slack"
