importScripts('https://www.gstatic.com/firebasejs/10.12.0/firebase-app-compat.js');
importScripts('https://www.gstatic.com/firebasejs/10.12.0/firebase-messaging-compat.js');

firebase.initializeApp({
  apiKey: "AIzaSyAzvQmLbvJcywzoQUegfkYnzw0cSa8H7xU",
  authDomain: "geoloc-1574d.firebaseapp.com",
  projectId: "geoloc-1574d",
  storageBucket: "geoloc-1574d.firebasestorage.app",
  messagingSenderId: "790753354879",
  appId: "1:790753354879:web:44d3d77f54f0af1e8d8b4a",
  measurementId: "G-SXY61NFPC8",
});

const messaging = firebase.messaging();

messaging.onBackgroundMessage((payload) => {
  const title = payload.notification?.title || 'Geoloc';
  const options = {
    body: payload.notification?.body || '',
    data: payload.data || {},
  };
  self.registration.showNotification(title, options);
});
