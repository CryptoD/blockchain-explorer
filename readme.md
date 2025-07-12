# Blockchain Explorer

This is a work-in-progress blockchain explorer project. It is a private project and not intended for use, copying, or publishing in any form. 

## About

The purpose of this project is to create a blockchain explorer that allows users to search for blocks, transactions, and addresses on various blockchain networks. 

## Project Status

This project is still in development and is not yet complete. The current focus is on building the search functionality and improving the user interface. 

## Architecture
- Frontend: React/Vue/Angular
- Backend: Node.js/Express 
- Database: PostgreSQL/MongoDB
- Blockchain Integration: Web3.js/Ethers.js

## Usage

As this is a private project, it is not available for public use. 

## Contributing

Contributions to this project are not accepted. 

## License

This project is not licensed and is not available for use, copying, or publishing.

## Project Setup

### Dependencies
This project uses Go modules. Key dependencies include:
- Gin web framework for routing.
- go-cache for in-memory caching.

To install dependencies, run:
```
go mod tidy
go mod download
```

### Environment Variables
Set the following variables before running the application:
- `baseURL`: The base URL for the GetBlock API.
- `apiKey`: Your API key for accessing the GetBlock API.

You can set them in your environment, e.g., using export commands:
```
export baseURL="https://your.api.endpoint"
export apiKey="your_api_key"
```

### Run Instructions
1. Ensure Go is installed (version 1.21 or higher).
2. Build and run the application using:
```
go build -o blockchain-explorer
./blockchain-explorer
```
Or, for development:
```
go run main.go
```

This will start the server and make the blockchain explorer available. 
