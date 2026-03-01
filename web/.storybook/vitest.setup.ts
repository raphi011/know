import { beforeAll } from "vitest";
import { setProjectAnnotations } from "@storybook/react-vite";
import preview from "./preview";

const annotations = setProjectAnnotations([preview.composed]);

beforeAll(annotations.beforeAll);
