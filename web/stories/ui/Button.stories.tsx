import preview from "#.storybook/preview";
import { PlusIcon, ArrowRightIcon, TrashIcon } from "@heroicons/react/20/solid";
import { Button } from "@/components/ui/button";

const meta = preview.meta({
  title: "UI/Button",
  component: Button,
  tags: ["autodocs"],
  parameters: { layout: "centered" },
  argTypes: {
    variant: {
      control: "select",
      options: ["primary", "outline", "destructive", "ghost"],
    },
    size: { control: "select", options: ["sm", "md", "lg"] },
    loading: { control: "boolean" },
    disabled: { control: "boolean" },
  },
});

export default meta;

export const Primary = meta.story({
  args: { children: "Primary Button" },
});

export const Outline = meta.story({
  args: { variant: "outline", children: "Outline" },
});

export const Destructive = meta.story({
  args: { variant: "destructive", children: "Delete" },
});

export const Ghost = meta.story({
  args: { variant: "ghost", children: "Ghost" },
});

export const Small = meta.story({
  args: { size: "sm", children: "Small" },
});

export const Large = meta.story({
  args: { size: "lg", children: "Large" },
});

export const Loading = meta.story({
  args: { loading: true, children: "Loading..." },
});

export const Disabled = meta.story({
  args: { disabled: true, children: "Disabled" },
});

export const WithIconLeft = meta.story({
  args: {
    children: (
      <>
        <PlusIcon />
        Create
      </>
    ),
  },
});

export const WithIconRight = meta.story({
  args: {
    children: (
      <>
        Continue
        <ArrowRightIcon />
      </>
    ),
  },
});

export const DestructiveWithIcon = meta.story({
  args: {
    variant: "destructive",
    children: (
      <>
        <TrashIcon />
        Delete
      </>
    ),
  },
});

export const AllVariants = meta.story({
  render: () => (
    <div className="flex flex-wrap items-center gap-3">
      <Button>Primary</Button>
      <Button variant="outline">Outline</Button>
      <Button variant="destructive">Destructive</Button>
      <Button variant="ghost">Ghost</Button>
    </div>
  ),
});

export const AllSizes = meta.story({
  render: () => (
    <div className="flex items-center gap-3">
      <Button size="sm">Small</Button>
      <Button size="md">Medium</Button>
      <Button size="lg">Large</Button>
    </div>
  ),
});
