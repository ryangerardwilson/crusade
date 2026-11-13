import QtQuick

Item {
    id: tile

    property string appName: ""
    property color accent: "#ffffff"
    property bool selected: false
    property int number: 0
    property bool showLabel: true
    property real tileSize: 88
    readonly property real badgeSize: Math.max(14, Math.min(20, tileSize * 0.24))
    readonly property real badgeMargin: Math.max(4, tileSize * 0.08)

    width: tileSize + 28
    height: tileSize + 42
    scale: selected ? 1.1 : 1
    opacity: selected ? 1 : 0.74

    Rectangle {
        id: halo

        anchors.horizontalCenter: parent.horizontalCenter
        anchors.top: parent.top
        width: tile.tileSize + 16
        height: width
        radius: width / 2
        color: "transparent"
        border.width: tile.selected ? 2 : 1
        border.color: tile.selected ? tile.accent : Qt.rgba(1, 1, 1, 0.2)
        opacity: tile.selected ? 0.78 : 0.44
    }

    Rectangle {
        id: button

        anchors.centerIn: halo
        width: tile.tileSize
        height: width
        radius: width / 2
        color: tile.selected ? Qt.rgba(1, 1, 1, 0.08) : Qt.rgba(0, 0, 0, 0.48)
        border.width: 1
        border.color: tile.selected ? tile.accent : Qt.rgba(1, 1, 1, 0.22)
    }

    Text {
        anchors.centerIn: button
        text: tile.appName.length > 0 ? tile.appName.charAt(0).toUpperCase() : "?"
        color: tile.selected ? tile.accent : "#ffffff"
        font.family: "JetBrainsMono Nerd Font"
        font.pixelSize: tile.tileSize * 0.36
        font.weight: Font.Bold
    }

    Text {
        anchors.right: button.right
        anchors.top: button.top
        anchors.rightMargin: tile.badgeMargin
        anchors.topMargin: tile.badgeMargin
        text: tile.number > 0 && tile.number < 10 ? tile.number : ""
        color: tile.selected ? "#000000" : "#ffffff"
        font.family: "JetBrainsMono Nerd Font"
        font.pixelSize: Math.max(7, tile.badgeSize * 0.5)
        font.weight: Font.Bold

        Rectangle {
            anchors.centerIn: parent
            width: tile.badgeSize
            height: tile.badgeSize
            radius: tile.badgeSize / 2
            color: tile.selected ? tile.accent : Qt.rgba(1, 1, 1, 0.1)
            z: -1
        }

    }

    Text {
        anchors.horizontalCenter: button.horizontalCenter
        anchors.top: button.bottom
        anchors.topMargin: 10
        visible: tile.showLabel
        width: parent.width
        horizontalAlignment: Text.AlignHCenter
        text: tile.appName.toUpperCase()
        color: tile.selected ? "#ffffff" : "#8d8d8d"
        elide: Text.ElideRight
        font.family: "JetBrainsMono Nerd Font"
        font.pixelSize: 10
        font.weight: Font.Bold
    }

    Behavior on scale {
        NumberAnimation {
            duration: 160
            easing.type: Easing.OutCubic
        }

    }

    Behavior on opacity {
        NumberAnimation {
            duration: 160
        }

    }

}
