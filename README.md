# Bitcoin Blockchain Explorer

A comprehensive web-based application for exploring the Bitcoin blockchain with real-time access to blocks, transactions, and address information.

## Table of Contents
- [Features](#features)
- [Architecture](#architecture)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Installation](#installation)
  - [Running the Application](#running-the-application)
- [API Documentation](#api-documentation)
- [Development](#development)
  - [Project Structure](#project-structure)
  - [Technology Stack](#technology-stack)
  - [Development Setup](#development-setup)
- [Documentation](#documentation)
- [Contributing](#contributing)
- [License](#license)

## Features

### Search Functionality
- **Block Search**: Search by block height or block hash
- **Transaction Search**: Search by transaction hash (TXID)
- **Address Search**: Search by Bitcoin address
- **Autocomplete**: Real-time suggestions as you type

### Real-time Data Display
- **Latest Blocks**: View the most recent blocks with key statistics
- **Latest Transactions**: See recent transaction activity
- **Network Status**: Current network statistics and metrics
- **Charts and Graphs**: Visual representation of network activity including:
  - Mempool size over time
  - Block time statistics
  - Transaction volume trends

### User Interface Features
- **Responsive Design**: Works on desktop, tablet, and mobile devices
- **Internationalization**: Multi-language support
- **Dark/Light Theme**: Comfortable viewing in different environments
- **Keyboard Navigation**: Full keyboard accessibility support

## Architecture

The Bitcoin Explorer follows a microservices-inspired architecture with clear separation of concerns:

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Frontend      │────▶│   Backend API   │────▶│  Blockchain     │
│  (HTML/JS/CSS)  │     │   (Go/Gin)      │     │  Node/API       │
└─────────────────┘     └─────────────────┘     └─────────────────┘
       │                        │                        │
       ▼                        ▼                        ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   CDN/Static    │     │   Redis Cache   │     │   PostgreSQL    │
│   Assets        │     │   (Caching)     │     │   (Persistence) │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

### Key Components
- **Frontend**: Single-page application with responsive design
- **Backend API**: RESTful API built with Go and Gin framework
- **Cache Layer**: Redis for performance optimization
- **Database**: PostgreSQL for persistent data storage
- **Blockchain Integration**: Direct RPC connection to Bitcoin node

## Getting Started

### Prerequisites
- Go 1.19+
- PostgreSQL 12+
- Redis 6+
- Node.js 16+ (for frontend development)
- Bitcoin node with RPC access (or access to a blockchain API)

### Installation

1. Clone the repository:
   ```bash
   git clone <repository-url>
   cd bitcoin-explorer
   ```

2. Install Go dependencies:
   ```bash
   go mod tidy
   go mod download
   ```

3. Set up the database:
   ```bash
   # Create database and run migrations
   # Update connection string in config
   ```

4. Configure environment variables:
   ```bash
   export BASE_URL="https://your.bitcoin.node.endpoint"
   export API_KEY="your-api-key"
   export DATABASE_URL="postgresql://user:password@localhost:5432/bitcoin_explorer"
   export REDIS_URL="redis://localhost:6379"
   ```

### Running the Application

```bash
go run main.go
```

The application will start on `http://localhost:8080`

## API Documentation

For detailed API documentation, see [API_DOCUMENTATION.md](API_DOCUMENTATION.md)

### Quick API Reference

Base URL: `http://localhost:8080/api`

#### Search Endpoints
- `GET /api/search?q={query}` - Search blocks, transactions, or addresses
- `GET /api/blocks/{hash_or_height}` - Get block details
- `GET /api/transactions/{txid}` - Get transaction details
- `GET /api/addresses/{address}` - Get address details

#### Response Format
All responses are in JSON format:

Success Response:
```json
{
  "success": true,
  "data": { ... },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

Error Response:
```json
{
  "success": false,
  "error": "Error message",
  "code": "ERROR_CODE",
  "timestamp": "2024-01-01T00:00:00Z"
}
```

## Development

### Project Structure
```
bitcoin-explorer/
├── main.go                 # Application entry point
├── go.mod                  # Go module definition
├── go.sum                  # Go dependencies
├── bitcoin.html            # Main frontend file
├── index.html              # Alternative frontend
├── src/                    # Frontend source files
│   ├── components/         # Reusable UI components
│   ├── pages/              # Page components
│   ├── services/           # API service clients
│   └── utils/              # Utility functions
├── internal/               # Internal packages
│   ├── api/                # API handlers and routes
│   ├── blockchain/         # Blockchain integration
│   ├── cache/              # Caching layer
│   ├── config/             # Configuration management
│   ├── database/           # Database access layer
│   └── utils/              # Internal utilities
├── docs/                   # Documentation files
├── scripts/                # Build and deployment scripts
└── tests/                  # Test files
```

### Technology Stack
- **Backend**: Go with Gin framework
- **Frontend**: HTML, CSS (Tailwind), JavaScript
- **Database**: PostgreSQL
- **Cache**: Redis
- **Blockchain Integration**: Bitcoin Core RPC
- **Deployment**: Docker, Kubernetes (optional)

### Development Setup

1. Install development dependencies:
   ```bash
   # Install Go tools
   go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
   
   # Install frontend dependencies
   npm install
   ```

2. Run tests:
   ```bash
   go test ./...
   ```

3. Run development server with hot reload:
   ```bash
   go run main.go
   ```

## Documentation

- [User Documentation](USER_DOCUMENTATION.md) - Complete guide for end users
- [API Documentation](API_DOCUMENTATION.md) - Detailed API reference for developers
- [Developer Documentation](DEVELOPER_DOCUMENTATION.md) - Technical documentation for contributors

## Contributing

We welcome contributions to improve the Bitcoin Explorer. Please see our [Contributing Guidelines](DEVELOPER_DOCUMENTATION.md#contributing-guidelines) for details on:

1. Code standards and practices
2. Pull request process
3. Testing requirements
4. Documentation standards

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.