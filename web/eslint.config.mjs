import nextConfig from "eslint-config-next";
import prettierConfig from "eslint-config-prettier";

const config = [
  { ignores: ["storybook-static/"] },
  ...nextConfig,
  prettierConfig,
];

export default config;
