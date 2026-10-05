//This is just a test for the electron setup

//setup
const { app, BrowserWindow } = require('electron');
const path = require('path');


function createWindow(){
    const window = new BrowserWindow({
        width:1000,
        height: 700
    }) ;
    //load test html
    window.loadFile(path.join(__dirname, '..', 'renderer', 'index.html'));
}

//create window when electron starts
app.whenReady().then(() => {createWindow();});

//close app on last window closed
app.on('window-all-closed', () => {app.quit();});