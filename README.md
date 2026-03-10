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

### User Interface Features
- **Responsive Design**: Works on desktop, tablet, and mobile devices
- **Internationalization**: Multi-language support
- **Dark/Light Theme**: Comfortable viewing in different environments
- **Keyboard Navigation**: Full keyboard accessibility support
- **Feedback Form**: Users can submit feedback directly from the homepage

## Architecture

The Bitcoin Explorer follows a microservices-inspired architecture with clear separation of concerns:

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   Frontend      │────▶│   Backend API   │────▶│  Blockchain     │
│  (HTML/JS/CSS)  │     │   (Go/Gin)      │     │  Node/API       │
└─────────────────┘     └─────────────────┘     └─────────────────┘
        │                        │                        │
        ▼                        ▼                        ▼
┌─────────────────┐     ┌─────────────────┐
│   CDN/Static    │     │   Redis Cache   │
│   Assets        │     │   (Caching)     │
└─────────────────┘     └─────────────────┘
```

### Key Components
- **Frontend**: Single-page application with responsive design
- **Backend API**: RESTful API built with Go and Gin framework
- **Cache Layer**: Redis for performance optimization
- **Blockchain Integration**: Direct RPC connection to Bitcoin node

## Getting Started

### Prerequisites
- Go 1.22+
- Redis 6+
- Node.js 16+ (for frontend development)
- Bitcoin node with RPC access (or access to a blockchain API)

### Installation
1. Clone the repository:
   ```bash
   git clone <repository-url>
   cd bitcoin-explorer
   ```

2. Copy environment file and configure:
   ```bash
   cp .env.example .env
   # Edit .env and set required values:
   # - POSTGRES_PASSWORD
   # - GETBLOCK_BASE_URL / GETBLOCK_ACCESS_TOKEN
   # - ADMIN_USERNAME / ADMIN_PASSWORD (especially for non-development environments)
   ```

3. Install frontend dependencies:
   ```bash
   npm install
   ```

4. Start the application with Docker Compose:
   ```bash
   docker-compose up -d
   ```

The application will start on `http://localhost:3000`, with Adminer available at `http://localhost:8080` for database administration.

### Alternative: Manual Setup

1. Install Go dependencies:
   ```bash
   go mod tidy
   go mod download
   ```

2. Configure environment variables:
   ```bash
   export APP_ENV=development                               # or staging/production
   export GETBLOCK_BASE_URL="https://your.bitcoin.node.endpoint"
   export GETBLOCK_ACCESS_TOKEN="your-api-key"
   export REDIS_URL="redis://localhost:6379"
   export ADMIN_USERNAME="admin"                            # for local dev only
   export ADMIN_PASSWORD="change_me_admin_password"         # for local dev only
   ```

3. Run the application:
   ```bash
   go run main.go
   ```

The application will start on `http://localhost:8080`

## API Documentation

For detailed API documentation, see [API_TEST_RESULTS.md](API_TEST_RESULTS.md)

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
bitcoin-explorer/
├── main.go                 # Application entry point
├── go.mod                  # Go module definition
├── go.sum                  # Go dependencies
├── bitcoin.html            # Main frontend file
├── index.html              # Alternative frontend
├── src/                    # Frontend source files
│   └── styles/             # Stylesheets
├── dist/                   # Built assets
├── docker-compose.yml      # Docker Compose configuration
├── Dockerfile              # Docker build configuration
├── package.json            # Frontend dependencies
├── API_TEST_RESULTS.md     # API testing documentation
├── README.md               # This file
└── LICENSE                 # License file
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
│   ├── cache/              # Caching layer (Redis)
│   ├── config/             # Configuration management
│   ├── blockchain/         # Blockchain integration (Bitcoin Core RPC)
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

### Admin credentials, environments, and rotation

- In **development** (`APP_ENV=development` or unset), the application will fall back to `admin` / `admin123` if `ADMIN_USERNAME` / `ADMIN_PASSWORD` are not provided. This is for local convenience only and must not be used in shared or production deployments.
- In any **non-development** environment (`APP_ENV` not equal to `development`), the app will **refuse to start** if `ADMIN_USERNAME` or `ADMIN_PASSWORD` are missing. Set these to strong, unique values and rotate them by updating the environment, restarting the app, and then changing the password through the UI if desired.

## Documentation

- [API Test Results](API_TEST_RESULTS.md) - API testing and validation results
## Contributing

We welcome contributions to improve the Bitcoin Explorer. Please follow standard Go and web development practices.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.