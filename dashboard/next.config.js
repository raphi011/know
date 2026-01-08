/** @type {import('next').NextConfig} */
const nextConfig = {
  output: 'standalone',
  async rewrites() {
    return [
      {
        source: '/api/graphql',
        destination: 'http://localhost:8080/graphql',
      },
    ];
  },
};

module.exports = nextConfig;
