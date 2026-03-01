import preview from "#.storybook/preview";
import { Checkbox } from "@/components/ui/checkbox";

const meta = preview.meta({
  title: "UI/Checkbox",
  component: Checkbox,
  tags: ["autodocs"],
  parameters: { layout: "centered" },
});

export default meta;

export const Default = meta.story({
  args: { label: "Accept terms and conditions" },
});

export const Checked = meta.story({
  args: { label: "Receive notifications", defaultChecked: true },
});

export const Disabled = meta.story({
  args: { label: "Disabled option", disabled: true },
});

export const DisabledChecked = meta.story({
  args: { label: "Locked selection", disabled: true, defaultChecked: true },
});

export const WithoutLabel = meta.story({
  args: { "aria-label": "Select row" },
});
