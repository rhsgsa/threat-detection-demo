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
    Flex,
    HStack,
    Badge,
    Button,
    ChakraProvider,
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
import theme from './theme/Card.js';

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
        setPhoto(data);
      }
    
      useEffect(() => {
          let data = (checked?annotatedImage:rawImage);
          if (data == null || data === '') {
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
        mt='4'
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
    const [promptlist, setPromptlist] = useState([]);
  
    useEffect(() => {
      fetch(baseurl + '/api/prompt')
        .then(response => response.json())
        .then(json => {
          setPromptlist(json);
        })
        .catch(error => console.error(error));
    }, []);
  
    let dropdownArr = [];
    for (let i=0; i<promptlist.length; i++) {
      dropdownArr.push(<option key={promptlist[i].id} value={promptlist[i].id}>{promptlist[i].prompt}</option>)
    }
  
   return dropdownArr
  }
  
  // Prompt dropdown menu and POST prompt to server
  function Dropdown ({ promptID }) {
    const toast = useToast();
  
    const handleChangePrompt = async (event) => {
      try {
        const newPromptID = event.target.value;
        const newPromptText = event.target.options[event.target.selectedIndex].text;

        const headers = new Headers();
        headers.append('Content-Type', 'application/json');
        const config = {headers: headers};
        let data = await axios.post(
          (baseurl + '/api/prompt'),
            JSON.stringify({
              id: parseInt(newPromptID),
            }),
          config
        );
        console.log("🚀 ~ handleChangePrompt ~ data:", data)
        
        toast({
          title: 'Prompt Changed to ' + newPromptText,
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
              onChange={handleChangePrompt}
              value={ promptID }
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
    return <textarea cols={70} rows={10} readOnly value={ response } />
  }

  function AI({ response, threat }) {
    const [ colour, setColour ] = useState('');
    console.log(threat)

    useEffect(() => {
      if (threat === '') {
        setColour('whiteborder')
      } else if (threat === 'Low') {
        setColour('greenborder')
      } else if (threat === 'Medium') {
        setColour('yellowborder')
      } else if (threat === 'High') {
        setColour('redborder')
      }
    }, [colour, threat]);

    return (
      <ChakraProvider theme={theme}>
        <Card w='100%' variant={ colour }>
          <CardBody>
            <textarea cols={70} rows={5} readOnly value={ response } />
          </CardBody>
        </Card>
      </ChakraProvider>
    )
  }

  function ThreatLevel({ threat }) {
    const [ colour, setColour ] = useState('');

    useEffect(() => {
      var threatUpper = threat.toUpperCase()
      console.log('"' + threat + '"')
      if (threatUpper === 'LOW') {
        setColour('green')
      } else if (threatUpper === 'MEDIUM') {
        setColour('yellow')
      } else if (threatUpper === 'HIGH') {
        setColour('red')
      } else {
        setColour('white')
      }
    }, [threat]);
    

    return <Badge variant='solid'  colorScheme={ colour } fontSize='0.8em'>{ threat }</Badge>
  }

function Dashboard2 () {
    const [ isLoaded, setIsLoaded ] = useState(true);
    const [ annotatedImage, setAnnotatedImage ] = useState('');
    const [ rawImage, setRawImage ] = useState('');
    const [ timestamp, setTimestamp ] = useState('');
    const [ prompt, setPrompt ] = useState(0);
    const [ llm_response, setLLMResponse ] = useState('');
    const [ ai_response, setAIResponse ] = useState('');
    const [ showButton, setShowButton ] = useState(true);

    useEffect(() => {
        const evtSource = new EventSource(baseurl + "/api/sse");
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
        setAIResponse('');
        });

        evtSource.addEventListener("prompt", event => {
        let obj = JSON.parse(event.data);
        if (obj == null || obj.id == null) return;
        setPrompt(obj.id);
        });

        evtSource.addEventListener("ollama_response_start", event => {
        setLLMResponse('');
        });

        evtSource.addEventListener("ollama_response", event => {
        const obj = JSON.parse(event.data);
        setLLMResponse(oldResponse => oldResponse + obj.response);
        });

        evtSource.addEventListener("openai_response_start", event => {
          setAIResponse('');
        });
  
        evtSource.addEventListener("openai_response", event => {
          const obj = JSON.parse(event.data);
          setAIResponse(oldResponse => oldResponse + obj.response);
        });

        evtSource.addEventListener("pause_events", event => {
          setShowButton(true);
        });
        
        evtSource.addEventListener("resume_events", event => {
          setShowButton(false);
        });

        fetch(baseurl + '/api/currentstate')
        .then(response => response.json())
        .then(json => {
          if (json == null) return;
          if (json.annotated_image != null && json.annotated_image !== "") setAnnotatedImage(json.annotated_image);
          if (json.raw_image != null && json.raw_image !== "") setRawImage(json.raw_image);
          if ((json.annotated_image != null && json.annotated_image !== "") || (json.raw_image != null && json.raw_image !== "")) setIsLoaded(true);
          if (json.timestamp != null) {
            let date = new Date(json.timestamp * 1000);
            setTimestamp(date.toString().split(' ')[4]);
          }
          if (json.prompt != null && json.prompt.id != null) setPrompt(json.prompt.id);
          if (json.image_analysis != null) setLLMResponse(json.image_analysis);
          if (json.threat_analysis != null) setAIResponse(json.threat_analysis);
          if (json.events_paused != null) setShowButton(json.events_paused);
        })
        .catch(error => console.error(error));
    }, []);

    //GET Display Data on Button Click
    function SubmitHandler(e) {
      e.preventDefault();
      const callAxios = async () => {
        await axios
          .get(baseurl + '/api/resumeevents')
          .then(response => {
            console.log('SUCCESS', response);
          })
          .catch(error => {
            console.log(error);
          });
      };
      callAxios();
      setIsLoaded(false);
      setLLMResponse('');
      setAIResponse('');
    }
    
  return (
    
    <Container maxW="8xl" centerContent>

    <HStack>
      <Center p={3} w="100%">
        <Box w='30%' p={4}>
          <Image objectFit='cover' src={ncsrhlogo} />
        </Box>
        <Heading size='lg'>Threat Detection Dashboard</Heading>
      </Center>
    </HStack>

    <Center>
        <Grid
          templateColumns="repeat(2, 1fr)"
          templateRows="repeat(1, 1fr)"
          gap={6}
          p={3}
          w="300%"
          bgColor="#eff7fa"
          m="0px 0 0px 0"
          borderRadius="lg"
          borderWidth="1px"
        >
            <GridItem colSpan={1} rowSpan={1}>
            <VStack spacing={4}>
              <Heading as='h3' size='md'>
                  Image Detection
                </Heading> 
                <Center>
                
                <Card w='100%'>
                    <CardBody>
                        <Stack mb='6' spacing='3'>  
                          <Timestamp timestamp={timestamp}/>
                          <PlaySound timestamp={timestamp}/>
                          <Skeleton
                            height='450px'
                            isLoaded={isLoaded}
                            fadeDuration={1}
                            fitContent
                          >
                            <Photo annotatedImage={annotatedImage} rawImage={rawImage}/>
                          </Skeleton>
                          <Button 
                            colorScheme='blue' 
                            isDisabled={!showButton}
                            onClick={SubmitHandler}
                          > 
                            Resume Stream 
                          </Button>
                        </Stack>
                    </CardBody>
                </Card>
                </Center>
                </VStack>
            </GridItem>

            <GridItem colSpan={1} rowSpan={1}>
            <VStack spacing={4}>
              <Heading as='h3' size='md'>
                Image Analysis
              </Heading>  
              <Dropdown promptID={prompt}/>
              <Divider orientation="horizontal" /> 
              <Card w='100%'>
                <CardBody>
                    <LLM response={llm_response.trim()}/>
                </CardBody>
              </Card>
              <Divider orientation="horizontal" />
              <Flex>
                <Heading as='h3' size='md' mr='4'>
                  Threat Level
                </Heading>
                <ThreatLevel threat={ai_response.split(' ').slice(1,2).toString()}/>
              </Flex>
              <AI response={ai_response.trim()} threat={ai_response.split(' ').slice(1,2).toString()}/>
            </VStack>
            </GridItem>   
        </Grid>
      </Center>
    </Container>
  )
}

export default Dashboard2