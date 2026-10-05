I want you to use the video.mp4 and extra the audio into a audio.wav file.

Then I want you to create a python directory where I would start doing signal analysis. The first python file I want it one that mixes a selected .wav file in the output directory with the audio.wav file. This python script should have two parameters hardcoded where one will be the signal file and the other one will be the noise file. The resulting file should only be as long as the shortest file. But we should also be able to add an offset the the first file.

Here is the absolute criterea:
- The python script should have two variables which will be hardcoded to data/audio.wav and the other to output/am-20261005-085414.883124000-3417.wav.
- There should then be another varaible which is hardcoded to 2 seconds
- We expect this python script to produce a approx. 6 second audio start with two seconds of the audio.wav and then the signal file in the output diretory.
- We expect a clean function which takes in the two pcm samples from the audio files and produces the new PCM values. The code for reading and saving the file, should be outside of this function.