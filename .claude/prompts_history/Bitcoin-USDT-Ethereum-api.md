Task:

Find a cryptocurrency exchange that provides an open public API for trading data for the following assets:
- USDT
- Bitcoin (BTC)
- Ethereum (ETH)

We need access to basic market and trading information, including:
- trade history / executed trades
- timestamp of each trade
- traded volume
- price
- order book / market depth
- other basic market data available through the public API

Requirements:
1. Identify a suitable exchange with an open API.
2. Briefly describe which endpoints or API methods provide:
   - trades
   - prices
   - order book data
   - timestamps
   - volumes
3. Confirm that the API is public and does not require private account access for this data.

After that, create a technical implementation plan for ingesting this data into PostgreSQL using Go workers.

The implementation plan should include:
- overall architecture
- how Go workers will fetch data from the exchange API
- how data should be parsed and normalized
- PostgreSQL schema ideas for storing:
  - trades
  - order book snapshots / depth
  - asset or symbol metadata
- batching / polling or streaming approach
- handling rate limits
- retry and error handling strategy
- idempotency / deduplication approach
- logging and monitoring considerations
- recommendations for scaling the solution if more trading pairs or exchanges are added later

