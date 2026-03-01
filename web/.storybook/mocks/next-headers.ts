const map = new Map<string, string>();

export function cookies() {
  return {
    get: (name: string) => ({ value: map.get(name) }),
    set: (name: string, value: string) => map.set(name, value),
    delete: (name: string) => map.delete(name),
  };
}

export function headers() {
  return new Headers();
}
