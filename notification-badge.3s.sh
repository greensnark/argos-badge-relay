#! /bin/sh

( cat ~/.config/argos/.notifications; echo "·" ) | perl -ne 'print if /\S/'
echo ---
