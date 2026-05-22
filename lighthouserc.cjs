// Lighthouse CI — performance budgets (Core Web Vitals). See docs/PERFORMANCE_BUDGETS.md
const baseURL = process.env.LHCI_BASE_URL || 'http://127.0.0.1:18080';

module.exports = {
  ci: {
    collect: {
      numberOfRuns: process.env.CI ? 1 : 3,
      startServerCommand: 'bash scripts/lhci-server.sh',
      startServerReadyPattern: 'Server is up',
      url: [
        `${baseURL}/`,
        `${baseURL}/bitcoin`,
        `${baseURL}/symbols`,
      ],
      settings: {
        preset: 'desktop',
        onlyCategories: ['performance', 'accessibility', 'best-practices'],
      },
    },
    assert: {
      assertions: {
        // Core Web Vitals — primary budgets (task 80)
        'largest-contentful-paint': ['warn', { maxNumericValue: 2500 }],
        'cumulative-layout-shift': ['error', { maxNumericValue: 0.1 }],
        // Supporting metrics (warn only — CI runners vary)
        'total-blocking-time': ['warn', { maxNumericValue: 300 }],
        'speed-index': ['warn', { maxNumericValue: 3400 }],
        'categories:performance': ['warn', { minScore: 0.75 }],
        'categories:accessibility': ['warn', { minScore: 0.9 }],
      },
    },
    upload: {
      target: 'filesystem',
      outputDir: '.lighthouseci',
    },
  },
};
