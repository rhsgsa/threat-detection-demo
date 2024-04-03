import {
    Container,
    Box,
    Image,
    Center,
    Divider,
    VStack,
    GridItem,
    Grid,
    Heading,
    Card,
    CardBody,
    Stack,
    Checkbox,
    Select,
    useToast,
    Skeleton, 
} from '@chakra-ui/react';

import {
    React, 
    useState, 
    useEffect,
    useRef,
} from 'react'

import axios from 'axios';
import ncsrhlogo from './ncs_rh_logo.jpg';
import threatAudio from './warning.mp3';

let baseurl = 'http://localhost:8080'

function Photo({ annotatedImage, rawImage }) {
    const [photo, setPhoto] = useState('');
    const [checked, setChecked] = useState(true);
    const [refresh, setRefresh] = useState(null);

      function clearPhoto() {
        var canvas = document.createElement('canvas');
        var context = canvas.getContext('2d');
        context.fillStyle = "#AAA";
        context.fillRect(0, 0, canvas.width, canvas.height);
      
        var data = canvas.toDataURL('image/png');
        setPhoto('data:image/jpeg;charset=utf-8;base64,' + data);
      }
    
      useEffect(() => {
          let data = (checked?annotatedImage:rawImage);
          if (data == null) {
            clearPhoto();
            return;
          }else{
              setPhoto('data:image/jpeg;charset=utf-8;base64,' + data);
          }
          setRefresh(false);
        }, [refresh, checked, annotatedImage, rawImage]);

    return (
    <Container>
        <Checkbox 
        colorScheme='blue' 
        defaultChecked
        onChange={(e) => setChecked(e.target.checked)}>Annotated
        </Checkbox>
        <Image 
        objectFit='cover' 
        borderRadius='10px' 
        src={photo} 
        alt="threatpic"/>
    </Container>)
}

function PlaySound ({timestamp}) {
  const [ sound, setSound ] = useState();
  const [checked, setChecked] = useState(false);
  const [refresh, setRefresh] = useState(null);
  const currentImageTimestamp = useRef(0);
  const playSound = useRef(true);
  const noSound = useRef(false);

  useEffect(() => {
    let alarm = new Audio(threatAudio);
    let data = (checked?playSound.current:noSound.current);
    if (data == null) {
      return;
    }else{
        setSound(data);
        if (sound === true && currentImageTimestamp.current !== timestamp) {
          console.log("currentImageTimestamp=" + currentImageTimestamp.current + " event.data=" + timestamp);
          currentImageTimestamp.current = timestamp;
          alarm.play();
        }
    }
    setRefresh(false);
  }, [timestamp, refresh, checked, sound])

  return (
    <Container>
        <Checkbox 
        colorScheme='blue' 
        onChange={(e) => setChecked(e.target.checked)}>Play Sound
        </Checkbox>
    </Container>)
}

// Process GET prompt list from server
function Promptlist () {
    const [promptlist, setPromptlist] = useState('');
  
    useEffect(() => {
      fetch(baseurl + '/api/prompt')
        .then(response => response.json())
        .then(json => setPromptlist(json))
        .catch(error => console.error(error));
    }, []);
  
    let dropdownArr = [];
    for (let i=0; i<promptlist.length; i++) {
      dropdownArr.push(<option value={promptlist[i]}>{promptlist[i]}</option>)
    }
  
   return dropdownArr
  }
  
  // Prompt dropdown menu and POST prompt to server
  function Dropdown () {
    const toast = useToast();
  
    const handleChangePrompt = async (event) => {
      try {
        const config = {
          headers: {
            'Content-type': 'application/json',
          },
        };
        let data = await axios.post(
          (baseurl + '/api/prompt'),
            JSON.stringify({
              prompt: event.target.value,
            }),
          config
        );
        console.log("🚀 ~ handleChangePrompt ~ data:", data)
        
        toast({
          title: 'Prompt Changed to ' + event.target.value,
          status: 'success',
          duration: 5000,
          isClosable: true,
          position: 'bottom',
        });
      } catch (error) {
        toast({
          title: 'Error Occurred!',
          description: error.response.data.message,
          status: 'error',
          duration: 5000,
          isClosable: true,
          position: 'bottom',
        });
      }
    };
    
    return  <Select 
              size='lg' 
              bg='white' 
              variant='outline' 
              placeholder='Select prompt'        
              onChange={handleChangePrompt}
            >
              <Promptlist/>
            </Select>
  }
  
  function Timestamp({ timestamp }) {
    return (
      <Container>
        <Heading color='blue.600' size='md'>Timestamp: {timestamp}</Heading>
      </Container>
      )
  }
  
  function LLM({ response }) {
    return <textarea cols={70} rows={15} readOnly value={ response } />
  }

function Dashboard2 () {
    const [ isLoaded, setIsLoaded ] = useState(true);
    const [ annotatedImage, setAnnotatedImage ] = useState('');
    const [ rawImage, setRawImage ] = useState('');
    const [ timestamp, setTimestamp ] = useState('');
    const [ response, setResponse ] = useState('');

    useEffect(() => {
        const evtSource = new EventSource("http://localhost:8080/api/sse");
        setIsLoaded(false);

        evtSource.addEventListener("annotated_image", event => {
        setAnnotatedImage(event.data);
        setIsLoaded(true);
        })

        evtSource.addEventListener("raw_image", event => {
        setRawImage(event.data);
        setIsLoaded(true);
        })

        evtSource.addEventListener("timestamp", event => {
        let date = new Date(event.data * 1000);
        setTimestamp(date.toString().split(' ')[4]);
        });

        evtSource.addEventListener("llm_response_start", event => {
        setResponse('');
        });

        evtSource.addEventListener("llm_response", event => {
        const obj = JSON.parse(event.data);
        setResponse(oldResponse => oldResponse + obj.response);
        });
    }, []);
    
  return (
    <Container maxW="8xl" centerContent>
    
    <Center p={3} w="100%" m="40px 0 15px 0">
        <Image objectFit='cover' src={ncsrhlogo} />  
    </Center>

    <Divider orientation="horizontal" />

    <Box m="20px"></Box>
    
    <Heading size='lg'>Threat Detection Dashboard</Heading>

    <Center>
        <Grid
          templateColumns="repeat(2, 1fr)"
          templateRows="repeat(1, 1fr)"
          gap={6}
          p={3}
          w="300%"
          bgColor="#eff7fa"
          m="40px 0 15px 0"
          borderRadius="lg"
          borderWidth="1px"
        >
            <GridItem colSpan={1} rowSpan={1}>
                <Center>
                <Card w='100%'>
                    <CardBody>
                        <Stack mb='6' spacing='3'>   
                          <Timestamp timestamp={timestamp}/>
                          <PlaySound timestamp={timestamp}/>
                          <Skeleton
                            height='345px'
                            isLoaded={isLoaded}
                            fadeDuration={1}
                          >
                            <Photo annotatedImage={annotatedImage} rawImage={rawImage}/>
                          </Skeleton>
                        </Stack>
                    </CardBody>
                </Card>
                </Center>
            </GridItem>

            <GridItem colSpan={1} rowSpan={1}>
            <VStack spacing={4}>
                <Dropdown/>
                <Divider orientation="horizontal" />   
                <Card>
                <CardBody>
                    <LLM response={response.trim()}/>
                </CardBody>
                </Card>
            </VStack>
            </GridItem>   
        </Grid>
      </Center>
    </Container>
  )
}

export default Dashboard2