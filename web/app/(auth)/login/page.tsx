import { LoginForm } from "./login-form";

const DEFAULT_SERVER_URL = "http://localhost:8484";

export default function LoginPage() {
  const defaultServerUrl = process.env.BACKEND_URL || DEFAULT_SERVER_URL;

  return <LoginForm defaultServerUrl={defaultServerUrl} />;
}
