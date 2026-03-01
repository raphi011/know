import preview from "#.storybook/preview";
import { Select } from "@/components/ui/select";

const meta = preview.meta({
  title: "UI/Select",
  component: Select,
  tags: ["autodocs"],
  args: { "aria-label": "Select option" },
  parameters: { layout: "centered" },
  decorators: [
    (Story) => (
      <div className="w-72">
        <Story />
      </div>
    ),
  ],
});

export default meta;

export const Default = meta.story({
  args: {
    children: (
      <>
        <option value="">Choose an option</option>
        <option value="option1">Option 1</option>
        <option value="option2">Option 2</option>
        <option value="option3">Option 3</option>
      </>
    ),
  },
});

export const WithValue = meta.story({
  args: {
    defaultValue: "option2",
    children: (
      <>
        <option value="option1">Option 1</option>
        <option value="option2">Option 2</option>
        <option value="option3">Option 3</option>
      </>
    ),
  },
});

export const Error = meta.story({
  args: {
    error: true,
    children: (
      <>
        <option value="">Select a category</option>
        <option value="blog">Blog</option>
        <option value="news">News</option>
      </>
    ),
  },
});

export const Disabled = meta.story({
  args: {
    disabled: true,
    defaultValue: "option1",
    children: (
      <>
        <option value="option1">Option 1</option>
        <option value="option2">Option 2</option>
      </>
    ),
  },
});
