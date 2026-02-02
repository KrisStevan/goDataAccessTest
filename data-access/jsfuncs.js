function searchArtist(){
    const artistInput = document.getElementById('artist');
    const artist = artistInput ? artistInput.value : '';

    if (artist.trim()) {
        window.location.href = '/view/' + encodeURIComponent(artist);
    } else {
        window.location.href = '/view/';
    }
}