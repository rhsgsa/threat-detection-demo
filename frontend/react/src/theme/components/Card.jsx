import { cardAnatomy } from '@chakra-ui/anatomy';
import { createMultiStyleConfigHelpers, defineStyle } from '@chakra-ui/react';

const { definePartsStyle, defineMultiStyleConfig } =
  createMultiStyleConfigHelpers(cardAnatomy.keys);

// define custom variant
  const variants = {
  greenborder: definePartsStyle({
    container: {
      borderColor: "#48BB78",
      borderWidth: "3px",
    }
  }),
  yellowborder: definePartsStyle({
    container: {
      borderColor: "#ECC94B",
      borderWidth: "3px",
    }
  }),
  redborder: definePartsStyle({
    container: {
      borderColor: "#F56565",
      borderWidth: "3px",
    }
  }),
  whiteborder: definePartsStyle({
    container: {
      borderColor: "#FFFFFF",
      borderWidth: "3px",
    }
  }),
  
};

// export variants in the component theme
export const cardTheme = defineMultiStyleConfig({ variants });