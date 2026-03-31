package server

import "github.com/CryptoD/blockchain-explorer/internal/repos"

// appRepos holds domain Redis repositories (wired in Run and TestMain after rdb is set).
var appRepos *repos.Stores
