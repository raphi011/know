import { useState } from "react";
import preview from "#.storybook/preview";
import { Dialog } from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const meta = preview.meta({
  title: "UI/Dialog",
  component: Dialog,
  tags: ["autodocs"],
  parameters: { layout: "centered" },
});

export default meta;

function SimpleDialogDemo() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>Open dialog</Button>
      <Dialog open={open} onClose={() => setOpen(false)} title="Confirmation">
        <p className="text-sm text-slate-600 dark:text-slate-400">
          Are you sure you want to perform this action?
        </p>
        <div className="mt-6 flex justify-end gap-3">
          <Button variant="outline" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button onClick={() => setOpen(false)}>Confirm</Button>
        </div>
      </Dialog>
    </>
  );
}

export const Simple = meta.story({
  render: () => <SimpleDialogDemo />,
});

function FormDialogDemo() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>With form</Button>
      <Dialog open={open} onClose={() => setOpen(false)} title="Invite user">
        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="name" required>
              Name
            </Label>
            <Input id="name" placeholder="Jane Doe" />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="email" required>
              Email
            </Label>
            <Input id="email" type="email" placeholder="jane@example.com" />
          </div>
        </div>
        <div className="mt-6 flex justify-end gap-3">
          <Button variant="outline" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button onClick={() => setOpen(false)}>Invite</Button>
        </div>
      </Dialog>
    </>
  );
}

export const WithForm = meta.story({
  render: () => <FormDialogDemo />,
});
