import preview from "#.storybook/preview";
import { within, userEvent, expect, fn } from "storybook/test";
import { Toast } from "@/components/ui/toast";

const meta = preview.meta({
  title: "UI/Toast",
  component: Toast,
  tags: ["autodocs"],
  parameters: { layout: "centered" },
  args: {
    onClose: fn(),
  },
  argTypes: {
    variant: {
      control: "select",
      options: ["success", "error", "info"],
    },
  },
  decorators: [
    (Story) => (
      <div className="w-80">
        <Story />
      </div>
    ),
  ],
});

export default meta;

export const Success = meta.story({
  args: {
    variant: "success",
    title: "Post published",
    description: "Your post has been published successfully.",
  },
  play: async ({ canvasElement, args }) => {
    const canvas = within(canvasElement);

    // Verify toast content renders
    await expect(canvas.getByText("Post published")).toBeInTheDocument();
    await expect(
      canvas.getByText("Your post has been published successfully."),
    ).toBeInTheDocument();

    // Click close button
    await userEvent.click(canvas.getByRole("button"));
    await expect(args.onClose).toHaveBeenCalled();
  },
});

export const Error = meta.story({
  args: {
    variant: "error",
    title: "Error",
    description: "The action could not be completed.",
  },
});

export const Info = meta.story({
  args: {
    variant: "info",
    title: "New update",
    description: "A new version is available.",
  },
});

export const WithAction = meta.story({
  args: {
    variant: "success",
    title: "Post deleted",
    action: { label: "Undo", onClick: fn() },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);

    // Verify action button is visible
    const actionBtn = canvas.getByRole("button", { name: /undo/i });
    await expect(actionBtn).toBeInTheDocument();
    await userEvent.click(actionBtn);
  },
});

export const AllVariants = meta.story({
  render: () => (
    <div className="space-y-3">
      <Toast
        variant="success"
        title="Success"
        description="Everything went well."
        onClose={fn()}
      />
      <Toast
        variant="error"
        title="Error"
        description="Something went wrong."
        onClose={fn()}
      />
      <Toast
        variant="info"
        title="Info"
        description="New update available."
        onClose={fn()}
      />
    </div>
  ),
});
