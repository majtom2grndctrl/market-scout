import { Stack } from "@/components/primitives/stack";
import { Text } from "@/components/primitives/text";
import { Button } from "@/components/ui/button";

export default function Home() {
  return (
    <Stack
      as="main"
      minHeight="svh"
      direction="col"
      align="center"
      justify="center"
      gap={400}
    >
      <Text as="h1" size={600} weight={600}>
        Welcome to Next.js
      </Text>
      <Button>Get started</Button>
    </Stack>
  );
}
