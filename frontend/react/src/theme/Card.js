import { extendTheme } from '@chakra-ui/react';
import { cardTheme } from './components/Card.jsx';

const theme = extendTheme({
  components: {
    Card: cardTheme,
  },
});

export default theme;