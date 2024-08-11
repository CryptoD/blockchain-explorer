document.querySelector('#search-input').addEventListener('keydown', function (e) {
    if (e.key === 'Enter') {
      performSearch(e.target.value);
    }
  });
  
  document.querySelector('#search-icon').addEventListener('click', function () {
    const query = document.querySelector('#search-input').value;
    performSearch(query);
  });
  
  function performSearch(query) {
    fetch('/api/search?q=' + encodeURIComponent(query))
      .then(function (response) {
        if (response.ok) {
          return response.json();
        } else {
          throw new Error('Failed to fetch data from the API');
        }
      })
      .then(function (data) {
        displaySearchResults(data);
      })
      .catch(function (error) {
        console.error('Error:', error);
        alert('An error occurred while searching the blockchain. Please try again later.');
      });
  }
  
  function displaySearchResults(data) {
    const resultDiv = document.createElement('div');
    resultDiv.innerHTML = '<pre>' + JSON.stringify(data, null, 2) + '</pre>';
    document.body.appendChild(resultDiv);
  }