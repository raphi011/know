import preview from "#.storybook/preview";
import { FormField } from "@/components/form-field";

const meta = preview.meta({
  title: "Composites/FormField",
  component: FormField,
  tags: ["autodocs"],
  parameters: { layout: "centered" },
  decorators: [
    (Story) => (
      <div className="w-80">
        <Story />
      </div>
    ),
  ],
});

export default meta;

export const Default = meta.story({
  args: {
    label: "Email",
    placeholder: "name@example.com",
    type: "email",
  },
});

export const Required = meta.story({
  args: {
    label: "Name",
    placeholder: "Jane Doe",
    required: true,
  },
});

export const WithError = meta.story({
  args: {
    label: "Email",
    placeholder: "name@example.com",
    type: "email",
    error: "Please enter a valid email address.",
    required: true,
  },
});

export const TextArea = meta.story({
  args: {
    label: "Description",
    placeholder: "Write a short description...",
    type: "textarea" as const,
  },
});

export const SelectField = meta.story({
  args: {
    label: "Category",
    type: "select" as const,
    children: (
      <>
        <option value="">Choose a category</option>
        <option value="blog">Blog</option>
        <option value="news">News</option>
        <option value="tutorial">Tutorial</option>
      </>
    ),
  },
});

export const Disabled = meta.story({
  args: {
    label: "Read-only field",
    disabled: true,
    value: "Cannot edit this",
  },
});

export const AllStates = meta.story({
  render: () => (
    <div className="space-y-4">
      <FormField label="Default" placeholder="Enter text" />
      <FormField label="Required" placeholder="Required field" required />
      <FormField
        label="With error"
        placeholder="name@example.com"
        type="email"
        error="This field is required."
        required
      />
      <FormField label="Disabled" placeholder="Not editable" disabled />
    </div>
  ),
});
