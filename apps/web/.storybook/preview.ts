import type { Preview } from "@storybook/react";
import { withThemeByClassName } from "@storybook/addon-themes";
import { Geist } from "next/font/google";
import { createElement } from "react";
import "../app/globals.css";

const geist = Geist({ subsets: ["latin"], variable: "--font-sans" });

const preview: Preview = {
  parameters: {
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/i,
      },
    },
    a11y: {
      config: {},
      options: {},
    },
  },
  decorators: [
    withThemeByClassName({
      themes: {
        light: "",
        dark: "dark",
      },
      defaultTheme: "light",
    }),
    (Story) => createElement("div", { className: `font-sans ${geist.variable}` }, createElement(Story)),
  ],
};

export default preview;
