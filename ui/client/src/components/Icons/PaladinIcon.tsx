interface PaladinIconProps {
  className?: string;
  width?: number;
  height?: number;
}

export const PaladinIcon: React.FC<PaladinIconProps> = ({
  className = "",
  width = 64,
  height = 64,
}) => {
  return (
    <img
      src={"/paladin-icon-light.svg"}
      width={width}
      height={height}
      className={className}
      alt="Paladin Icon"
    />
  );
};
