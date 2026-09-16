const test=require('node:test'),assert=require('node:assert/strict');
const {format,parse}=require('../internal/web/static/video-times.js');
test('video timestamps round trip including frame precision and hour-long videos',()=>{for(const seconds of [0,0.001,0.1,9,10,59.999,60,130.125,1799.999,3600,86400])assert.equal(parse(format(seconds)),seconds);assert.equal(format(130),'2:10');assert.equal(parse('1:02:03.125'),3723.125);});
test('timestamps reject ambiguous or invalid input',()=>{for(const input of ['','-1','1:99','1:2:3:4','1.2345','Infinity','NaN','<script>'])assert.ok(Number.isNaN(parse(input)),input);});
