module.exports = {
  content: [
    './*.html',
    './static/js/**/*.js',
  ],
  theme: {
    extend: {
      colors: {
        primary: 'var(--color-primary)',
        accent: 'var(--color-accent)',
        bg: 'var(--color-bg)',
        text: 'var(--color-text)',
        'bg-secondary': 'var(--color-bg-secondary)',
        'text-secondary': 'var(--color-text-secondary)',
        'border': 'var(--color-border)',
        'hover': 'var(--color-hover)'
      }
    }
  },
  plugins: []
}
