import type { Config } from 'tailwindcss'

const config: Config = {
  content: [
    './app/**/*.{js,ts,jsx,tsx,mdx}',
    './components/**/*.{js,ts,jsx,tsx,mdx}',
  ],
  theme: {
    extend: {
      colors: {
        primary: {
          DEFAULT: '#13b6ec',
          dark: '#0d9bc8',
        },
        'background-dark': '#101d22',
        'surface-dark': '#1a2c35',
        'text-secondary': '#8b9da5',
      },
      animation: {
        'page-in': 'pageIn 0.3s ease-out',
      },
      keyframes: {
        pageIn: {
          '0%': { opacity: '0', transform: 'translateY(10px)' },
          '100%': { opacity: '1', transform: 'translateY(0)' },
        },
      },
    },
  },
  plugins: [],
}

export default config
