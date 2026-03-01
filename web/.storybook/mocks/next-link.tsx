import type { ComponentPropsWithoutRef } from "react";

export default function Link({
  children,
  ...props
}: ComponentPropsWithoutRef<"a">) {
  return <a {...props}>{children}</a>;
}
