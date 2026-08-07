import type { Meta, StoryObj } from "@storybook/react";
import { Button } from "./button";
import { Plus, ArrowRight, Trash2 } from "lucide-react";

const meta: Meta<typeof Button> = {
  title: "UI/Button",
  component: Button,
  tags: ["autodocs"],
  argTypes: {
    variant: {
      control: "select",
      options: [
        "default",
        "outline",
        "secondary",
        "ghost",
        "destructive",
        "link",
      ],
    },
    size: {
      control: "select",
      options: [
        "default",
        "xs",
        "sm",
        "lg",
        "icon",
        "icon-xs",
        "icon-sm",
        "icon-lg",
      ],
    },
    disabled: {
      control: "boolean",
    },
  },
  args: {
    children: "Button",
    variant: "default",
    size: "default",
    disabled: false,
  },
};

export default meta;
type Story = StoryObj<typeof Button>;

export const Default: Story = {
  args: {
    children: "Default Button",
  },
};

export const Secondary: Story = {
  args: {
    variant: "secondary",
    children: "Secondary Button",
  },
};

export const Outline: Story = {
  args: {
    variant: "outline",
    children: "Outline Button",
  },
};

export const Ghost: Story = {
  args: {
    variant: "ghost",
    children: "Ghost Button",
  },
};

export const Destructive: Story = {
  args: {
    variant: "destructive",
    children: "Destructive Button",
  },
};

export const Link: Story = {
  args: {
    variant: "link",
    children: "Link Button",
  },
};

export const WithIconStart: Story = {
  args: {
    children: (
      <>
        <Plus className="size-4" />
        Create Item
      </>
    ),
  },
};

export const WithIconEnd: Story = {
  args: {
    children: (
      <>
        Continue
        <ArrowRight className="size-4" />
      </>
    ),
  },
};

export const IconButton: Story = {
  args: {
    size: "icon",
    children: <Trash2 className="size-4" />,
    "aria-label": "Delete item",
  },
};

export const Disabled: Story = {
  args: {
    disabled: true,
    children: "Disabled Button",
  },
};
