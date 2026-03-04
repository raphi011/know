import { env } from "@/app/lib/env";
import { LoginForm } from "./login-form";

export default function LoginPage() {
  return <LoginForm defaultServerUrl={env.BACKEND_URL} />;
}
